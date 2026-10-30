package aiimage

import (
	"crypto/rand"
	dbsql "database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/imagegen"
	sql "github.com/FloatTech/sqlite"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Integer micro-dollars avoid rounding races. The budget is an admission quota,
// based on conservative estimates, not an OpenAI billing balance.
const usd = int64(1000000)

var billingZone = time.FixedZone("Asia/Shanghai", 8*60*60)

type renderSettings struct {
	ID          int64  `db:"id"`
	Quality     string `db:"quality"`
	Size        string `db:"size"`
	Budget      int64  `db:"budget"`
	CustomCost  string `db:"customCost"`
	OptionsJSON string `db:"options"`
}
type budgetEntry struct {
	ID      string `db:"id"`
	Month   string `db:"month"`
	Amount  int64  `db:"amount"`
	State   string `db:"state"`
	Model   string `db:"model"`
	Quality string `db:"quality"`
	Size    string `db:"size"`
}

func defaultSettings() renderSettings {
	return renderSettings{ID: 1, Quality: "medium", Size: "1024x1024", Budget: 100 * usd}
}
func (s *storage) settingsLocked() (renderSettings, error) {
	var v renderSettings
	err := s.db.Find("render_settings", &v, "WHERE id = 1")
	if errors.Is(err, sql.ErrNullResult) {
		return defaultSettings(), nil
	}
	if err != nil {
		return v, err
	}
	if !validQuality(v.Quality) || !validSize(v.Size) || v.Budget < 0 || v.Budget > 100000*usd {
		return v, fmt.Errorf("画图质量或预算配置损坏，请在后台重新核对")
	}
	return v, nil
}
func validQuality(v string) bool {
	return v == "low" || v == "medium" || v == "high" || v == "xhigh" || v == "max" || v == "auto"
}
func validSize(v string) bool {
	if v == "auto" {
		return true
	}
	_, _, err := dimensions(v)
	return err == nil
}

var dollarsRE = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,6})?$`)

func parseDollars(v string, max int64) (int64, error) {
	if len(v) > 20 || !dollarsRE.MatchString(v) {
		return 0, fmt.Errorf("金额须为非负美元数字，最多6位小数")
	}
	parts := strings.SplitN(v, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > max/usd {
		return 0, fmt.Errorf("金额超出范围")
	}
	fraction := int64(0)
	if len(parts) == 2 {
		fraction, _ = strconv.ParseInt(parts[1]+strings.Repeat("0", 6-len(parts[1])), 10, 64)
	}
	total := whole*usd + fraction
	if total > max {
		return 0, fmt.Errorf("金额超出范围")
	}
	return total, nil
}
func dollars(v int64) string        { return fmt.Sprintf("%d.%06d", v/usd, v%usd) }
func monthKey(now time.Time) string { return now.In(billingZone).Format("2006-01") }
func (s *storage) budgetLocked(now time.Time, limit int64) (core.BudgetStatus, error) {
	out := core.BudgetStatus{Month: monthKey(now), Timezone: "Asia/Shanghai"}
	database, err := s.ledger()
	if err != nil {
		return out, err
	}
	defer database.Close()
	rows, err := database.Query("SELECT amount,state FROM image_budget WHERE month = ?", out.Month)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	var total, pending int64
	for rows.Next() {
		var row budgetEntry
		if err = rows.Scan(&row.Amount, &row.State); err != nil {
			return out, err
		}
		if row.Amount < 0 || row.Amount > 100000*usd {
			return out, fmt.Errorf("预算记录异常")
		}
		total += row.Amount
		out.Requests++
		if row.State != "completed" {
			pending += row.Amount
			out.Uncertain++
		}
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	remaining := limit - total
	if remaining < 0 {
		remaining = 0
	}
	out.ChargedUSD = dollars(total)
	out.UncertainUSD = dollars(pending)
	out.RemainingUSD = dollars(remaining)
	out.Stopped = total >= limit
	return out, nil
}

func parseImagePrompt(prompt, defaultSize string) (string, string, error) {
	v := defaultSettings()
	v.Size = defaultSize
	clean, v, err := parseImageOptions(prompt, v)
	return clean, v.Size, err
}
func officialEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "api.openai.com" && u.Path == "/v1/images/generations" && u.RawQuery == "" && u.User == nil
}
func modelFamily(model string) string {
	switch model {
	case "gpt-image-1.5", "gpt-image-1.5-2025-12-16":
		return "gpt-image-1.5"
	case "gpt-image-1-mini":
		return model
	case "gpt-image-2", "gpt-image-2-2026-04-21":
		return "gpt-image-2"
	case "gpt-image-2.5-flare", "gpt-image-2.5-flare-2026-09-08",
		"gpt-image-2.5-sunburst", "gpt-image-2.5-sunburst-2026-09-08":
		return "gpt-image-2.5"
	default:
		return ""
	}
}
func quoteSingleImage(c imageConfig, mode, prompt string, v renderSettings) (int64, string, error) {
	if u, e := url.Parse(c.APIURL); e == nil && strings.EqualFold(u.Hostname(), "api.openai.com") && (!officialEndpoint(c.APIURL) || mode != "openai") {
		return 0, "", fmt.Errorf("OpenAI官方画图请使用openai类型及标准https生成地址")
	}
	if mode == "openai" && officialEndpoint(c.APIURL) {
		if v.CustomCost != "" {
			return 0, "", fmt.Errorf("OpenAI官方接口不能使用自定义费用绕过预算估价")
		}
		family := modelFamily(c.ModelName)
		if family == "gpt-image-2.5" || family == "gpt-image-2" {
			tokens, err := imageOutputTokens(family, v.Quality, v.Size)
			if err != nil {
				return 0, "", err
			}
			o, _ := optionsFor(v)
			output := (tokens + int64(o.PartialImages)*100) * 30
			amount := (output*120+99)/100 + (int64(len(prompt))+1024)*5
			return amount, "官方计算器估价（含流式预览）+20%输出余量+文本输入预留；auto按最高档/最大像素预留，非账单", nil
		}
		// Published output prices, USD micro-units; dimensions and quality are fixed.
		prices := map[string][3][2]int64{
			"gpt-image-1-mini": {{5000, 6000}, {11000, 15000}, {36000, 52000}},
			"gpt-image-1.5":    {{9000, 13000}, {34000, 50000}, {133000, 200000}},
		}
		table, ok := prices[family]
		if !ok {
			return 0, "", fmt.Errorf("当前模型尚无已核实的预算估价，已停止调用；请使用已支持的GPT Image模型或已核实的快照名")
		}
		quality := v.Quality
		if quality == "auto" {
			quality = "high"
		}
		q := map[string]int{"low": 0, "medium": 1, "high": 2}[quality]
		shape := 0
		if v.Size != "1024x1024" {
			shape = 1
		}
		// Reserve a 20% output margin plus a byte-based input allowance. Byte count
		// overestimates normal UTF-8 BPE tokens; 1024 extra tokens cover overhead.
		rate := int64(5)
		if family == "gpt-image-1-mini" {
			rate = 2
		}
		o, _ := optionsFor(v)
		outputRate := int64(32)
		if family == "gpt-image-1-mini" {
			outputRate = 8
		}
		amount := (table[q][shape]+int64(o.PartialImages)*100*outputRate)*120/100 + (int64(len(prompt))+1024)*rate
		return amount, "官方单张价格 + 20%输出余量 + 提示词输入预留（估算，非账单）", nil
	}
	if v.CustomCost == "" {
		return 0, "", fmt.Errorf("兼容或本地接口价格未知，已停止调用；请在后台填写服务商确认的每张最高总价，免费本地服务明确填0")
	}
	amount, err := parseDollars(v.CustomCost, 10000*usd)
	if err != nil {
		return 0, "", err
	}
	return amount, "管理员指定的每张最高总价（含输入、预览及服务费）", nil
}
func (s *storage) reserve(prompt string, now time.Time, refs ...referenceImage) (c imageConfig, mode string, v renderSettings, clean string, entry budgetEntry, err error) {
	s.Lock()
	defer s.Unlock()
	c, mode, err = s.readLocked()
	if err != nil {
		return
	}
	v, err = s.settingsLocked()
	if err != nil {
		return
	}
	if !validImageURL(c.APIURL) || c.ModelName == "" || (mode != "openai" && mode != "siliconflow") {
		err = fmt.Errorf("请先在后台配置画图接口和模型")
		return
	}
	clean, v, err = parseImageOptions(prompt, v)
	if err != nil {
		return
	}
	amount, _, e := quoteImage(c, mode, clean, v)
	if e != nil {
		err = e
		return
	}
	extra, e := referenceBudget(c, mode, v, refs)
	if e != nil {
		err = e
		return
	}
	amount += extra
	budget, e := s.budgetLocked(now, v.Budget)
	if e != nil {
		err = fmt.Errorf("预算读取失败，未调用模型")
		return
	}
	remaining, e := parseDollars(budget.RemainingUSD, 100000*usd)
	if e != nil {
		err = e
		return
	}
	if budget.Stopped || amount > remaining {
		err = fmt.Errorf("本月画图预算不足，已停止生成（剩余$%s，本次需预留$%s；北京时间下月重置）", budget.RemainingUSD, dollars(amount))
		return
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return
	}
	entry = budgetEntry{ID: hex.EncodeToString(id[:]), Month: monthKey(now), Amount: amount, State: "reserved", Model: c.ModelName, Quality: v.Quality, Size: v.Size}
	database, e := s.ledger()
	if e != nil {
		err = e
		return
	}
	defer database.Close()
	result, e := database.Exec("INSERT INTO image_budget (id,month,amount,state,model,quality,size) SELECT ?,?,?,?,?,?,? WHERE ? <= ? - COALESCE((SELECT SUM(amount) FROM image_budget WHERE month = ?),0) AND COALESCE((SELECT SUM(amount) FROM image_budget WHERE month = ?),0) < ?", entry.ID, entry.Month, amount, entry.State, entry.Model, entry.Quality, entry.Size, amount, v.Budget, entry.Month, entry.Month, v.Budget)
	if e != nil {
		err = fmt.Errorf("预算预留写入失败，未调用模型")
		return
	}
	n, e := result.RowsAffected()
	if e != nil || n != 1 {
		err = fmt.Errorf("本月画图预算已被其他请求占用，已停止生成")
	}
	return
}
func (s *storage) finish(entry budgetEntry, success bool) error {
	s.Lock()
	defer s.Unlock()
	state := "uncertain"
	if success {
		state = "completed"
	}
	// No refund on network failure or malformed replies: the remote may have billed.
	database, err := s.ledger()
	if err != nil {
		return err
	}
	defer database.Close()
	_, err = database.Exec("UPDATE image_budget SET state = ? WHERE id = ? AND state = 'reserved'", state, entry.ID)
	return err
}

// Keep accounting separate from configuration backups so restoring settings
// cannot restore a lower monthly balance.
func (s *storage) ledger() (*dbsql.DB, error) {
	if s.path == "" {
		return nil, fmt.Errorf("画图配置尚未初始化")
	}
	database, err := dbsql.Open(sql.DriverName, s.path+".budget.sqlite")
	if err != nil {
		return nil, err
	}
	if _, err = database.Exec("CREATE TABLE IF NOT EXISTS image_budget (id TEXT PRIMARY KEY,month TEXT NOT NULL,amount INTEGER NOT NULL CHECK(amount>=0),state TEXT NOT NULL,model TEXT NOT NULL,quality TEXT NOT NULL,size TEXT NOT NULL)"); err != nil {
		database.Close()
		return nil, err
	}
	return database, nil
}

func quoteImage(c imageConfig, mode, prompt string, v renderSettings) (int64, string, error) {
	if err := validateOptions(c, mode, v); err != nil {
		return 0, "", err
	}
	amount, note, err := quoteSingleImage(c, mode, prompt, v)
	if err != nil {
		return 0, "", err
	}
	o, _ := optionsFor(v)
	return amount * int64(o.Count), note + "；按图片张数预留", nil
}
