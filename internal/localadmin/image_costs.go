package localadmin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/imagegen"
	sqlite "github.com/FloatTech/sqlite"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type imageCostConfig struct {
	Key       string `json:"key"`
	ProjectID string `json:"projectID"`
	APIKeyID  string `json:"apiKeyID"`
	ProxyURL  string `json:"proxyURL"`
}
type imageCostRow struct {
	Date     string       `json:"date"`
	Item     string       `json:"item"`
	Amount   string       `json:"amountUSD"`
	Quantity *json.Number `json:"quantity"`
	Unit     string       `json:"unit"`
}
type imageCostReport struct {
	Start     string         `json:"start"`
	End       string         `json:"end"`
	QueriedAt string         `json:"queriedAt"`
	ProjectID string         `json:"projectID"`
	APIKeyID  string         `json:"apiKeyID"`
	TotalUSD  string         `json:"totalUSD"`
	Rows      []imageCostRow `json:"rows"`
}
type imageCostView struct {
	Revision      string           `json:"revision"`
	KeyConfigured bool             `json:"keyConfigured"`
	ProjectID     string           `json:"projectID"`
	APIKeyID      string           `json:"apiKeyID"`
	ProxyURL      string           `json:"proxyURL"`
	Report        *imageCostReport `json:"report"`
}

func costRevision(c imageCostConfig) string {
	b, _ := json.Marshal(c)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func (s *Server) imageCostDB() (*sql.DB, error) {
	path := filepath.Join(s.cfg.DataDir, "image-costs.sqlite")
	if info, e := os.Lstat(path); errors.Is(e, os.ErrNotExist) {
		if e = writeNewPrivateFile(path, nil); e != nil {
			return nil, e
		}
	} else if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("费用配置文件不可用")
	}
	db, e := sql.Open(sqlite.DriverName, path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	if _, e = db.Exec(`CREATE TABLE IF NOT EXISTS settings(id INTEGER PRIMARY KEY,config TEXT NOT NULL,report TEXT NOT NULL); INSERT OR IGNORE INTO settings VALUES(1,'{}','null')`); e != nil {
		db.Close()
		return nil, e
	}
	return db, nil
}
func readImageCostConfig(db *sql.DB) (imageCostConfig, *imageCostReport, error) {
	var c imageCostConfig
	var r *imageCostReport
	var raw, report string
	e := db.QueryRow(`SELECT config,report FROM settings WHERE id=1`).Scan(&raw, &report)
	if e != nil {
		return c, r, e
	}
	if e = json.Unmarshal([]byte(raw), &c); e != nil {
		return c, r, e
	}
	e = json.Unmarshal([]byte(report), &r)
	return c, r, e
}
func costView(c imageCostConfig, r *imageCostReport) imageCostView {
	return imageCostView{costRevision(c), c.Key != "", c.ProjectID, c.APIKeyID, c.ProxyURL, r}
}

var costID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)

func (s *Server) imageCostSettings(w http.ResponseWriter, r *http.Request) {
	s.costMu.Lock()
	defer s.costMu.Unlock()
	db, e := s.imageCostDB()
	if e != nil {
		apiError(w, 500, "费用配置读取失败")
		return
	}
	defer db.Close()
	c, report, e := readImageCostConfig(db)
	if e != nil {
		apiError(w, 500, "费用配置读取失败")
		return
	}
	if r.Method == http.MethodPost {
		var body struct {
			Revision  string      `json:"revision"`
			ProjectID string      `json:"projectID"`
			APIKeyID  string      `json:"apiKeyID"`
			ProxyURL  string      `json:"proxyURL"`
			Key       AIKeyChange `json:"key"`
		}
		if !readJSONLimit(w, r, &body, 8192) {
			return
		}
		if body.Revision != costRevision(c) {
			apiError(w, 409, "费用配置已变化，请重新读取")
			return
		}
		next := c
		next.ProjectID = strings.TrimSpace(body.ProjectID)
		next.APIKeyID = strings.TrimSpace(body.APIKeyID)
		next.ProxyURL = strings.TrimSpace(body.ProxyURL)
		if !strings.HasPrefix(next.ProjectID, "proj_") || !costID.MatchString(next.ProjectID) || (next.APIKeyID != "" && (!costID.MatchString(next.APIKeyID) || strings.HasPrefix(next.APIKeyID, "sk-"))) {
			apiError(w, 400, "请填写项目ID；可选填写API Key ID（不是密钥）")
			return
		}
		if next.ProxyURL != "" {
			u, e := url.Parse(next.ProxyURL)
			if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
				apiError(w, 400, "代理须为不含凭据的HTTP/HTTPS地址")
				return
			}
		}
		key, err := body.Key.apply(c.Key)
		if err != nil {
			apiError(w, 400, err.Error())
			return
		}
		if body.Key.Action == "replace" {
			if strings.TrimSpace(key) == "" || len(key) > 4096 || strings.ContainsAny(key, "\r\n") {
				apiError(w, 400, "请填写有效Admin Key")
				return
			}
			key = strings.TrimSpace(key)
		}
		next.Key = key
		if costRevision(next) != costRevision(c) {
			backup := filepath.Join(s.cfg.DataDir, "image-costs-backup-"+time.Now().UTC().Format("20060102T150405.000000000")+".sqlite")
			if e = writeNewPrivateFile(backup, nil); e != nil {
				apiError(w, 500, "费用配置备份失败")
				return
			}
			if _, e = db.Exec(`VACUUM INTO ?`, backup); e != nil {
				apiError(w, 500, "费用配置备份失败")
				return
			}
			raw, _ := json.Marshal(next)
			if _, e = db.Exec(`UPDATE settings SET config=?,report='null' WHERE id=1`, string(raw)); e != nil {
				apiError(w, 500, "费用配置保存失败")
				return
			}
			c = next
			report = nil
		}
	}
	jsonReply(w, 200, costView(c, report))
}

func (s *Server) imageCostQuery(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Revision string `json:"revision"`
		Start    string `json:"start"`
		End      string `json:"end"`
	}
	if !readJSONLimit(w, r, &body, 2048) {
		return
	}
	start, e1 := time.Parse("2006-01-02", body.Start)
	end, e2 := time.Parse("2006-01-02", body.End)
	if e1 != nil || e2 != nil || !end.After(start) || end.Sub(start) > 31*24*time.Hour || end.After(time.Now().UTC().Truncate(24*time.Hour).Add(24*time.Hour)) {
		apiError(w, 400, "按UTC日期查询1–31天，结束日期不包含在内")
		return
	}
	s.costMu.Lock()
	db, e := s.imageCostDB()
	if e != nil {
		s.costMu.Unlock()
		apiError(w, 500, "费用配置不可用")
		return
	}
	c, _, e := readImageCostConfig(db)
	db.Close()
	s.costMu.Unlock()
	if e != nil {
		apiError(w, 500, "费用配置不可用")
		return
	}
	if body.Revision != costRevision(c) {
		apiError(w, 409, "费用配置已变化")
		return
	}
	if c.Key == "" || !costID.MatchString(c.ProjectID) {
		apiError(w, 409, "请先配置独立Admin Key和项目ID")
		return
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if c.ProxyURL != "" {
		proxy, _ := url.Parse(c.ProxyURL)
		tr.Proxy = http.ProxyURL(proxy)
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	report, e := fetchImageCosts(ctx, client, c, start, end)
	if e != nil {
		apiError(w, 502, e.Error())
		return
	}
	s.costMu.Lock()
	defer s.costMu.Unlock()
	db, e = s.imageCostDB()
	if e != nil {
		apiError(w, 500, "费用结果保存失败")
		return
	}
	defer db.Close()
	current, _, e := readImageCostConfig(db)
	if e != nil || costRevision(current) != body.Revision {
		apiError(w, 409, "查询期间配置已变化，结果未保存")
		return
	}
	raw, _ := json.Marshal(report)
	if _, e = db.Exec(`UPDATE settings SET report=? WHERE id=1`, string(raw)); e != nil {
		apiError(w, 500, "费用结果保存失败")
		return
	}
	jsonReply(w, 200, costView(c, &report))
}

// Fixed official endpoint: Admin credentials never follow drawing/local endpoint settings.
func fetchImageCosts(ctx context.Context, client *http.Client, c imageCostConfig, start, end time.Time) (imageCostReport, error) {
	report := imageCostReport{Start: start.Format("2006-01-02"), End: end.Format("2006-01-02"), QueriedAt: time.Now().UTC().Format(time.RFC3339), ProjectID: c.ProjectID, APIKeyID: c.APIKeyID, Rows: []imageCostRow{}}
	total := new(big.Rat)
	page := ""
	seen := map[string]bool{}
	for n := 0; n < 20; n++ {
		q := url.Values{"start_time": {fmt.Sprint(start.Unix())}, "end_time": {fmt.Sprint(end.Unix())}, "bucket_width": {"1d"}, "limit": {"31"}, "group_by": {"line_item", "project_id"}, "project_ids": {c.ProjectID}}
		if c.APIKeyID != "" {
			q.Set("api_key_ids", c.APIKeyID)
		}
		if page != "" {
			q.Set("page", page)
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.openai.com/v1/organization/costs?"+q.Encode(), nil)
		req.Header.Set("Authorization", "Bearer "+c.Key)
		resp, e := client.Do(req)
		if e != nil {
			return report, fmt.Errorf("费用查询连接失败或超时，未取得账单")
		}
		raw, e := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return report, fmt.Errorf("费用接口HTTP %d；401/403请检查Admin Key及组织权限，429请稍后查询", resp.StatusCode)
		}
		if e != nil || len(raw) > 4<<20 {
			return report, fmt.Errorf("费用响应读取失败或过大")
		}
		var data struct {
			Data []struct {
				Start   int64 `json:"start_time"`
				End     int64 `json:"end_time"`
				Results []struct {
					Amount struct {
						Value    json.Number `json:"value"`
						Currency string      `json:"currency"`
					} `json:"amount"`
					Item     string       `json:"line_item"`
					Project  string       `json:"project_id"`
					Quantity *json.Number `json:"quantity"`
					Unit     string       `json:"quantity_unit"`
				} `json:"results"`
			} `json:"data"`
			More bool   `json:"has_more"`
			Next string `json:"next_page"`
		}
		if json.Unmarshal(raw, &data) != nil || data.Data == nil {
			return report, fmt.Errorf("费用接口返回无效JSON")
		}
		for _, bucket := range data.Data {
			if bucket.Start < start.Unix() || bucket.End > end.Unix() || bucket.End <= bucket.Start {
				return report, fmt.Errorf("费用响应日期范围不一致")
			}
			for _, row := range bucket.Results {
				if !strings.HasPrefix(row.Item, "gpt-image-") {
					continue
				}
				if row.Project != c.ProjectID || row.Amount.Currency != "usd" {
					return report, fmt.Errorf("费用响应项目或币种不一致")
				}
				amount, parseErr := core.ParseProviderUSD(string(row.Amount.Value))
				if parseErr != nil || len(row.Item) > 256 || len(row.Unit) > 40 {
					return report, fmt.Errorf("费用响应金额无效")
				}
				total.Add(total, amount)
				report.Rows = append(report.Rows, imageCostRow{Date: time.Unix(bucket.Start, 0).UTC().Format("2006-01-02"), Item: row.Item, Amount: amount.FloatString(9), Quantity: row.Quantity, Unit: row.Unit})
				if len(report.Rows) > 10000 {
					return report, fmt.Errorf("费用明细过多，请缩短日期范围")
				}
			}
		}
		if !data.More {
			report.TotalUSD = total.FloatString(9)
			return report, nil
		}
		if data.Next == "" || seen[data.Next] || len(data.Next) > 2048 {
			return report, fmt.Errorf("费用分页异常，未保存部分结果")
		}
		seen[data.Next] = true
		page = data.Next
	}
	return report, fmt.Errorf("费用分页过多，请缩短日期范围")
}
