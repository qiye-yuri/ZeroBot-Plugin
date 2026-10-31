package ctb

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

type Config struct {
	Enabled         bool   `json:"enabled"`
	Executable      string `json:"executable"`
	OutputDir       string `json:"outputDir"`
	TimeoutMinutes  int    `json:"timeoutMinutes"`
	Threads         int    `json:"threads"`
	NoProxy         bool   `json:"noProxy"`
	Upload          bool   `json:"upload"`
	GroupAccess     string `json:"groupAccess"`
	GroupDelivery   string `json:"groupDelivery"`
	GroupDailyLimit int    `json:"groupDailyLimit"`
}
type Request struct {
	Action  string `json:"action"`
	Pack    string `json:"pack"`
	Version string `json:"version"`
	Mode    string `json:"mode"`
}
type Result struct {
	Text   string   `json:"text"`
	Files  []string `json:"files"`
	Upload bool     `json:"upload"`
	TaskID string   `json:"taskId,omitempty"`
}
type Snapshot struct {
	Config   Config `json:"config"`
	Revision string `json:"revision"`
	Busy     bool   `json:"busy"`
	Last     string `json:"last"`
}
type Service struct {
	mu          sync.Mutex
	Path        string
	busy        bool
	last        string
	cancel      context.CancelFunc
	activeGroup int64
}

var Default = &Service{Path: filepath.Join(".local", "ctb", "config.json")}

func Defaults() Config {
	dir, _ := filepath.Abs(filepath.Join(".local", "ctb", "downloads"))
	return Config{OutputDir: dir, TimeoutMinutes: 30, Threads: 4, GroupDelivery: "group", GroupDailyLimit: 5}
}
func (c Config) Validate() error {
	if c.GroupDelivery != "group" && c.GroupDelivery != "private" {
		return fmt.Errorf("群聊文件发送位置必须为group/private")
	}
	if c.GroupDailyLimit < 1 || c.GroupDailyLimit > 100 {
		return fmt.Errorf("每群每天下载次数为1～100")
	}
	switch c.GroupAccess {
	case "", "disabled", "admin", "all", "owner":
	default:
		return fmt.Errorf("群聊权限必须为disabled/admin/all/owner")
	}
	if !filepath.IsAbs(c.Executable) || !strings.EqualFold(filepath.Ext(c.Executable), ".exe") {
		return fmt.Errorf("程序必须为绝对路径的exe文件")
	}
	if st, e := os.Stat(c.Executable); e != nil || st.IsDir() {
		return fmt.Errorf("程序文件不存在")
	}
	if !filepath.IsAbs(c.OutputDir) {
		return fmt.Errorf("下载目录必须为绝对路径")
	}
	if c.TimeoutMinutes < 1 || c.TimeoutMinutes > 120 || c.Threads < 1 || c.Threads > 16 {
		return fmt.Errorf("超时1～120分钟，并行数1～16")
	}
	return nil
}
func (s *Service) read() (Config, error) {
	c := Defaults()
	b, e := os.ReadFile(s.Path)
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	return c, e
}
func revision(c Config) string { b, _ := json.Marshal(c); return fmt.Sprintf("%x", sha256.Sum256(b)) }
func (s *Service) Settings(rev string, next *Config) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, e := s.read()
	if e != nil {
		return Snapshot{}, fmt.Errorf("配置读取失败")
	}
	if next != nil {
		if s.busy {
			return Snapshot{}, fmt.Errorf("任务执行中，请完成后修改")
		}
		if rev != revision(c) {
			return Snapshot{}, fmt.Errorf("配置已变更，请重新读取")
		}
		if e = next.Validate(); e != nil {
			return Snapshot{}, e
		}
		if e = core.Save(filepath.Join(filepath.Dir(s.Path), "backups", time.Now().Format("20060102-150405.000000000")+".json"), c); e != nil {
			return Snapshot{}, fmt.Errorf("备份失败")
		}
		if e = core.Save(s.Path, next); e != nil {
			return Snapshot{}, fmt.Errorf("保存失败")
		}
		c = *next
	}
	return Snapshot{c, revision(c), s.busy, s.last}, nil
}

var commandContext = exec.CommandContext

var digits = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)

func Args(r Request) ([]string, error) {
	switch r.Action {
	case "search":
		if len([]rune(r.Pack)) < 1 || len([]rune(r.Pack)) > 80 || strings.HasPrefix(r.Pack, "-") || strings.ContainsAny(r.Pack, "\r\n\x00") {
			return nil, fmt.Errorf("搜索关键词1～80字，不能以减号开头")
		}
		return []string{"search", r.Pack}, nil
	case "inspect", "download":
		if !digits.MatchString(r.Pack) {
			return nil, fmt.Errorf("整合包ID必须为正整数")
		}
		a := []string{r.Action, r.Pack}
		if r.Action == "inspect" {
			return a, nil
		}
		if r.Version != "" {
			if !digits.MatchString(r.Version) {
				return nil, fmt.Errorf("版本ID必须为正整数")
			}
			a = append(a, r.Version)
		}
		switch r.Mode {
		case "light":
			a = append(a, "--light")
		case "full":
		default:
			return nil, fmt.Errorf("下载模式：light/full")
		}
		return a, nil
	}
	return nil, fmt.Errorf("不支持的操作")
}

type limitedLog struct {
	mu sync.Mutex
	b  []byte
}

func (w *limitedLog) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	w.b = append(w.b, p...)
	if len(w.b) > 65536 {
		w.b = append([]byte(nil), w.b[len(w.b)-65536:]...)
	}
	return n, nil
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// The Windows CLI uses the system Chinese code page when stdout is piped.
// Keep UTF-8 output intact; decode legacy output before sending JSON to OneBot.
func readableOutput(b []byte) string {
	if !utf8.Valid(b) {
		if decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(b); err == nil {
			b = decoded
		}
	}
	s := ansi.ReplaceAllString(strings.ToValidUTF8(string(b), "�"), "")
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		border := line != ""
		for _, r := range line {
			if r != ' ' && (r < '─' || r > '╿') {
				border = false
				break
			}
		}
		if border || strings.HasPrefix(line, "按任意键退出") {
			continue
		}
		if strings.HasPrefix(line, "│") && strings.HasSuffix(line, "│") {
			cells := strings.Split(strings.Trim(line, "│"), "│")
			for i := range cells {
				cells[i] = strings.TrimSpace(cells[i])
			}
			line = strings.Join(cells, " | ")
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (s *Service) Run(ctx context.Context, r Request) (result Result, err error) {
	return s.RunFor(ctx, r, 0)
}

func (s *Service) RunFor(ctx context.Context, r Request, groupID int64) (result Result, err error) {
	return s.runFor(ctx, r, groupID, nil)
}

// RunWithDelivery keeps a successful download and its enabled initial upload
// in the same busy period. The caller owns the upload timeout and receipt.
func (s *Service) RunWithDelivery(ctx context.Context, r Request, groupID int64, deliver func(Result, string) error) (Result, error) {
	return s.runFor(ctx, r, groupID, deliver)
}

func (s *Service) runFor(ctx context.Context, r Request, groupID int64, deliver func(Result, string) error) (result Result, err error) {
	args, err := Args(r)
	if err != nil {
		return result, err
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return result, fmt.Errorf("已有任务执行中，请稍后重试")
	}
	c, e := s.read()
	if e != nil {
		s.mu.Unlock()
		return result, fmt.Errorf("配置读取失败")
	}
	if !c.Enabled {
		s.mu.Unlock()
		return result, fmt.Errorf("FTB工具未启用，请在后台开启")
	}
	if e = c.Validate(); e != nil {
		s.mu.Unlock()
		return result, e
	}
	if r.Action == "download" && groupID > 0 {
		if e = s.reserveGroupDownload(groupID, c.GroupDailyLimit, time.Now()); e != nil {
			s.mu.Unlock()
			return result, e
		}
	}
	timeout := time.Duration(c.TimeoutMinutes) * time.Minute
	if r.Action != "download" {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	s.cancel, s.activeGroup = cancel, groupID
	s.busy = true
	s.last = "正在执行 " + r.Action
	s.mu.Unlock()
	defer cancel()
	dir := ""
	sending := false
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		// Completion and cancellation share the lock: an accepted cancellation
		// cannot race a successful return that would start QQ file delivery.
		if !sending && ctx.Err() != nil && err == nil {
			err = taskContextError(ctx, dir)
		}
		s.cancel, s.activeGroup = nil, 0
		s.busy = false
		if err != nil {
			s.last = "失败：" + err.Error()
		} else {
			s.last = "完成：" + r.Action + " " + r.Pack
		}
		s.last = time.Now().Format(time.RFC3339) + " " + s.last
	}()
	if c.NoProxy {
		args = append(args, "--no-proxy")
	}
	if r.Action == "download" {
		if err = os.MkdirAll(c.OutputDir, 0700); err != nil {
			return result, fmt.Errorf("无法创建下载目录")
		}
		dir, err = os.MkdirTemp(c.OutputDir, "ctb-"+r.Pack+"-")
		if err != nil {
			return result, err
		}
		args = append(args, "--thread", fmt.Sprint(c.Threads), "--output", dir)
	}
	cmd := commandContext(ctx, c.Executable, args...)
	hideWindow(cmd)
	if dir != "" {
		cmd.Dir = dir
	}
	var out limitedLog
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	text := readableOutput(out.b)
	result.Text = text
	if ctx.Err() != nil {
		return result, taskContextError(ctx, dir)
	}
	if err != nil {
		if runes := []rune(text); len(runes) > 1500 {
			text = string(runes[len(runes)-1500:])
		}
		return result, fmt.Errorf("工具执行失败：%s", text)
	}
	if r.Action == "download" {
		entries, e := os.ReadDir(dir)
		if e != nil {
			return result, e
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return result, taskContextError(ctx, dir)
			}
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".zip") {
				continue
			}
			p := filepath.Join(dir, entry.Name())
			prepared, _, e := PrepareArchive(p)
			if e != nil {
				return result, e
			}
			result.Files = append(result.Files, prepared)
		}
		if len(result.Files) == 0 {
			return result, fmt.Errorf("命令结束但没有zip；请检查本地目录 %s", dir)
		}
		if ctx.Err() != nil {
			return result, taskContextError(ctx, dir)
		}
		if err = recordDelivery(dir, result.Files); err != nil {
			return result, fmt.Errorf("重发记录保存失败，文件已保留本机：%w", err)
		}
		result.TaskID = filepath.Base(dir)
		result.Text = "下载并校验完成：\n" + strings.Join(result.Files, "\n")
		result.Upload = c.Upload
	}
	if len([]rune(result.Text)) > 5000 {
		result.Text = string([]rune(result.Text)[:5000]) + "\n（结果过长已截断，请缩小搜索范围）"
	}
	if r.Action == "download" && result.Upload && deliver != nil {
		s.mu.Lock()
		if ctx.Err() != nil {
			s.mu.Unlock()
			return result, taskContextError(ctx, dir)
		}
		// Atomically stop accepting download cancellation before starting QQ upload.
		s.cancel = nil
		sending = true
		s.last = "正在发送 download " + r.Pack
		s.mu.Unlock()
		err = deliver(result, c.GroupDelivery)
	}
	return result, err
}
