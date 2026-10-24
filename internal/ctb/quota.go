package ctb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
)

type groupUsage struct {
	Date   string         `json:"date"`
	Counts map[string]int `json:"counts"`
}

// Called with s.mu held. Reserve before starting the process, so failed attempts
// and restarts cannot bypass the daily resource limit.
func (s *Service) reserveGroupDownload(groupID int64, limit int, now time.Time) error {
	path := filepath.Join(filepath.Dir(s.Path), "group-usage.json")
	day := now.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02")
	u := groupUsage{}
	if b, err := os.ReadFile(path); err == nil {
		if err = json.Unmarshal(b, &u); err != nil {
			return fmt.Errorf("群下载计数损坏，请联系主人检查；本次未开始下载")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("群下载计数读取失败；本次未开始下载")
	}
	if u.Date != day {
		u = groupUsage{Date: day}
	}
	if u.Counts == nil {
		u.Counts = map[string]int{}
	}
	key := strconv.FormatInt(groupID, 10)
	if u.Counts[key] >= limit {
		return fmt.Errorf("本群今天已达到%d次下载上限。下一步：北京时间明天00:00后重试；搜索和版本查询仍可使用", limit)
	}
	u.Counts[key]++
	if err := core.Save(path, u); err != nil {
		return fmt.Errorf("群下载计数保存失败；本次未开始下载")
	}
	return nil
}
