package updatewatch

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func init() { core.RuntimeAdmin = AdminSettings }

func revision(c core.Config) string {
	b, _ := json.Marshal(c)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// AdminSettings never waits behind a network/model request.
func AdminSettings(expected string, next *core.Config) (out core.AdminSnapshot, err error) {
	if !mu.TryLock() {
		return out, fmt.Errorf("正在检查来源或模型，请稍后重试；草稿仍保留")
	}
	defer mu.Unlock()
	c, err := readConfig()
	if err != nil {
		return out, fmt.Errorf("监控配置读取失败")
	}
	s, err := core.LoadState(statePath)
	if err != nil {
		return out, fmt.Errorf("监控状态读取失败")
	}
	if next != nil {
		if expected != revision(c) {
			return out, fmt.Errorf("配置已被其他页面或私聊修改，请重新加载后再保存")
		}
		if err = next.Validate(); err != nil {
			return out, err
		}
		next.ScheduleChangedAt = c.ScheduleChangedAt
		if next.DailyTime != c.DailyTime {
			next.ScheduleChangedAt = time.Now()
		}
		owner, e := resolveOwner(*next)
		if e != nil {
			return out, e
		}
		_, bot := pickBot(next.BotID)
		if bot == 0 {
			return out, fmt.Errorf("请选择明确的机器人账号")
		}
		if len(s.Pending) > 0 && (s.OwnerID != owner || s.BotID != bot) {
			return out, fmt.Errorf("旧对象仍有待发通知，投递完成后才能切换通知对象或机器人")
		}
		if next.Enabled {
			cfg, e := loadModelConfig()
			if e != nil || !cfg.ForModel(next.ModelRole).Ready() {
				return out, fmt.Errorf("请先在 AI 设置中配置所选模型入口")
			}
		}
		backup := filepath.Join(filepath.Dir(configPath), "backups", time.Now().Format("20060102-150405.000000000")+".json")
		if err = core.Save(backup, c); err != nil {
			return out, fmt.Errorf("旧配置备份失败，未保存")
		}
		if err = core.Save(configPath, *next); err != nil {
			return out, fmt.Errorf("监控配置保存失败")
		}
		c = *next
	}
	out.Config, out.Revision = c, revision(c)
	out.Owners = append([]int64{}, zero.BotConfig.SuperUsers...)
	out.Bots = []int64{}
	zero.RangeBot(func(id int64, _ *zero.Ctx) bool { out.Bots = append(out.Bots, id); return true })
	if cfg, e := loadModelConfig(); e == nil {
		for _, role := range []string{"", "GPT", "Summary", "Agent"} {
			m := cfg.ForModel(role)
			out.Models = append(out.Models, core.ModelOption{Role: role, Name: m.ModelName, Ready: m.Ready()})
		}
	}
	out.LastCheck, out.Pending, out.Baseline = s.LastCheck, len(s.Pending), len(s.Initialized)
	out.LastError, out.SourceErrors = s.LastError, s.SourceErrors
	return out, nil
}
