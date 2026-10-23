// Package updatewatch monitors verified public development sources.
package updatewatch

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	Enabled           bool      `json:"enabled"`
	IntervalHours     int       `json:"intervalHours"`
	DailyTime         string    `json:"dailyTime,omitempty"`
	ScheduleChangedAt time.Time `json:"scheduleChangedAt,omitempty"`
	OwnerID           int64     `json:"ownerId"`
	BotID             int64     `json:"botId"`
	ModelRole         string    `json:"modelRole"`
	CurseForge        bool      `json:"curseForge"`
	FTB               bool      `json:"ftb"`
	GitHub            bool      `json:"github"`
	Repositories      []string  `json:"repositories"`
	Announcements     []string  `json:"announcements"`
}

func DefaultConfig() Config {
	return Config{IntervalHours: 24, ModelRole: "Summary", CurseForge: true, FTB: true, GitHub: true, Repositories: []string{"FTBTeam/FTB-Evo-Companion"}, Announcements: []string{}}
}
func (c Config) Validate() error {
	if c.DailyTime != "" {
		if _, err := time.Parse("15:04", c.DailyTime); err != nil || len(c.DailyTime) != 5 {
			return fmt.Errorf("每日时间必须为HH:MM（北京时间）")
		}
	}
	if c.IntervalHours < 1 || c.IntervalHours > 168 {
		return fmt.Errorf("检查间隔必须为1～168小时")
	}
	if c.OwnerID < 0 || c.BotID < 0 {
		return fmt.Errorf("账号不能为负数")
	}
	switch c.ModelRole {
	case "", "GPT", "Summary", "Agent":
	default:
		return fmt.Errorf("模型入口仅支持 Chat、GPT、Summary、Agent（Chat在文件中为空字符串）")
	}
	if !c.CurseForge && !c.FTB && !c.GitHub && len(c.Announcements) == 0 {
		return fmt.Errorf("至少启用一个来源")
	}
	if len(c.Repositories) > 8 || len(c.Announcements) > 10 {
		return fmt.Errorf("最多8个官方仓库和10个公告网页")
	}
	for _, r := range c.Repositories {
		if !regexp.MustCompile(`^FTBTeam/[A-Za-z0-9_.-]+$`).MatchString(r) {
			return fmt.Errorf("仅支持FTBTeam官方仓库")
		}
	}
	for _, u := range c.Announcements {
		if err := allowedURL(u); err != nil {
			return err
		}
		p, _ := url.Parse(u)
		if p.Hostname() != "www.feed-the-beast.com" && p.Hostname() != "feed-the-beast.com" {
			return fmt.Errorf("公告请使用FTB官网公开转载页面；不支持Discord邀请页")
		}
	}
	return nil
}

func allowedURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return fmt.Errorf("仅允许官方HTTPS地址")
	}
	switch strings.ToLower(u.Hostname()) {
	case "www.curseforge.com", "www.feed-the-beast.com", "feed-the-beast.com", "api.feed-the-beast.com", "api.github.com", "github.com":
		return nil
	}
	return fmt.Errorf("来源域名不在官方白名单")
}

type Event struct {
	Key     string   `json:"key"`
	Date    string   `json:"date"`
	Version string   `json:"version,omitempty"`
	Stage   string   `json:"stage"`
	Text    string   `json:"text"`
	URLs    []string `json:"urls"`
}

func fingerprint(e Event) string { b, _ := json.Marshal(e); return fmt.Sprintf("%x", sha256.Sum256(b)) }

type Notice struct {
	Keys []string `json:"keys"`
	Text string   `json:"text"`
}
type State struct {
	Seen         map[string]Event  `json:"seen"`
	Initialized  map[string]bool   `json:"initialized"`
	Pending      []Notice          `json:"pending"`
	History      []Notice          `json:"history"`
	LastCheck    time.Time         `json:"lastCheck"`
	LastAttempt  time.Time         `json:"lastAttempt"`
	LastError    string            `json:"lastError"`
	SourceErrors map[string]string `json:"sourceErrors"`
	OwnerID      int64             `json:"ownerId"`
	BotID        int64             `json:"botId"`
}

func LoadState(path string) (State, error) {
	s := State{Seen: map[string]Event{}, Initialized: map[string]bool{}, SourceErrors: map[string]string{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	if s.Seen == nil || s.Initialized == nil {
		return s, fmt.Errorf("监控状态损坏")
	}
	return s, nil
}
func Save(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".updatewatch-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
