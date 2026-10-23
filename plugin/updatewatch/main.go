// Package updatewatch registers configurable official-development monitoring.
package updatewatch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FloatTech/ZeroBot-Plugin/internal/modelrequest"
	"github.com/FloatTech/ZeroBot-Plugin/internal/modelstats"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/chat"
	"github.com/FloatTech/zbputils/control"
	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	log "github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var engine = control.AutoRegister(&ctrl.Options[*zero.Ctx]{Brief: "官方开发进展监控（FTB Evolution 2）", Help: "仅主人私聊：更新监控状态 / 更新监控配置 / 更新监控测试通知 / 更新监控开启 / 更新监控暂停 / 更新监控检查 / 更新监控间隔 24 / 更新监控模型 Summary\n完整配置可在后台“更新监控”调整；默认关闭，首次建立静默基线。"})
var mu sync.Mutex
var loadModelConfig = chat.CurrentConfig
var inspectModelReply = func(string) {}
var configPath = filepath.Join(".local", "updatewatch", "config.json")
var statePath = filepath.Join(".local", "updatewatch", "state.json")

func readConfig() (core.Config, error) {
	c := core.DefaultConfig()
	b, e := os.ReadFile(configPath)
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return c, e
	}
	return c, c.Validate()
}
func resolveOwner(c core.Config) (int64, error) {
	if c.OwnerID == 0 {
		if len(zero.BotConfig.SuperUsers) == 1 {
			return zero.BotConfig.SuperUsers[0], nil
		}
		return 0, fmt.Errorf("需要配置唯一通知主人")
	}
	for _, id := range zero.BotConfig.SuperUsers {
		if id == c.OwnerID {
			return id, nil
		}
	}
	return 0, fmt.Errorf("接收人必须为已配置主人")
}
func pickBot(id int64) (*zero.Ctx, int64) {
	if id != 0 {
		return zero.GetBot(id), id
	}
	var selected *zero.Ctx
	var selectedID int64
	count := 0
	zero.RangeBot(func(id int64, c *zero.Ctx) bool { count++; selected = c; selectedID = id; return true })
	if count != 1 {
		return nil, 0
	}
	return selected, selectedID
}
func judge(role string) core.Judge {
	return func(ctx context.Context, changes []core.Change, history []core.Notice) ([]core.Notice, error) {
		cfg, err := loadModelConfig()
		if err != nil {
			return nil, fmt.Errorf("读取模型配置失败")
		}
		cfg = cfg.ForModel(role)
		if !cfg.Ready() {
			return nil, fmt.Errorf("所选模型入口未配置完整")
		}
		data, err := json.Marshal(struct {
			Changes []core.Change `json:"changes"`
			History []core.Notice `json:"history"`
		}{changes, history})
		if err != nil {
			return nil, err
		}
		if len(data) > 240000 {
			return nil, fmt.Errorf("待核验内容超过240KB，暂停自动判断，请检查状态")
		}
		p := cfg.RequestParams("")
		api := deepinfra.NewAPI(cfg.API, string(cfg.Key))
		api.SetHTTPClient(modelstats.Default.Client(&http.Client{Timeout: 90 * time.Second}, modelstats.GPT, modelstats.Labels{Model: cfg.ModelName}))
		var formatError error
		for attempt := 0; attempt < 2; attempt++ {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			protocol, err := cfg.Type.ProtocolForAPI(cfg.API, cfg.ModelName, 0.1, p.TopP, p.MaxN, p.ReasoningEffort, cfg.Separator)
			if err != nil {
				return nil, fmt.Errorf("所选模型协议不可用")
			}
			protocol.System(core.Rules)
			prompt := string(data)
			if attempt > 0 {
				prompt += "\n\n输出格式纠正：上次结果未通过格式或逐字证据检查。每个finding必须有indices、summary、impact、evidence四个字段；evidence不可省略，必须逐字复制上面text中的连续原文，至少12个字符。没有证据支持就返回空findings数组。仅输出合法JSON。"
			}
			reply, err := modelrequest.Request(api, protocol.User(model.NewContentText(prompt)))
			if err != nil {
				return nil, fmt.Errorf("模型核验失败，保留未处理变化")
			}
			inspectModelReply(reply)
			var notices []core.Notice
			notices, formatError = core.ParseDecision(reply, changes)
			if formatError == nil {
				return notices, nil
			}
		}
		return nil, formatError
	}
}

func run(force bool) error {
	if !mu.TryLock() {
		return fmt.Errorf("正在检查，请稍后查看状态")
	}
	defer mu.Unlock()
	c, err := readConfig()
	if err != nil {
		return err
	}
	if !c.Enabled && !force {
		return nil
	}
	owner, err := resolveOwner(c)
	if err != nil {
		return err
	}
	if !engine.IsEnabledIn(-owner) {
		return fmt.Errorf("插件在该主人私聊已停用")
	}
	s, err := core.LoadState(statePath)
	if err != nil {
		return err
	}
	bot, botID := pickBot(c.BotID)
	// Never transfer an old outbox to a newly configured recipient/bot.
	if len(s.Pending) > 0 && s.OwnerID != 0 && (s.OwnerID != owner || (botID != 0 && s.BotID != 0 && s.BotID != botID)) {
		return fmt.Errorf("旧对象仍有待发通知，投递完成后才能切换账号")
	}
	s.OwnerID = owner
	if botID != 0 {
		s.BotID = botID
	}
	send := func(text string) bool { return bot != nil && sendNotice(bot.CallAction, owner, text) }
	if err = core.Deliver(&s, statePath, send); err != nil {
		return err
	}
	if !force && !core.Due(c, s.LastCheck, time.Now()) {
		return nil
	}
	if !force && !s.LastAttempt.IsZero() && time.Since(s.LastAttempt) < 30*time.Minute {
		return nil
	}
	s.LastAttempt = time.Now()
	if err = core.Save(statePath, &s); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	log.Info("[updatewatch] 检查官方开发来源")
	result, failures := core.NewFetcher().Collect(ctx, c)
	if err = core.Check(ctx, &s, statePath, result, failures, judge(c.ModelRole)); err != nil {
		return err
	}
	log.Infof("[updatewatch] 检查完成：成功来源%d，失败来源%d，待发%d", len(result), len(failures), len(s.Pending))
	return core.Deliver(&s, statePath, send)
}

func init() {
	engine.OnPrefix("更新监控", zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		arg := strings.TrimSpace(ctx.State["args"].(string))
		if arg == "检查" {
			go func() {
				if e := run(true); e != nil {
					ctx.SendChain(message.Text("更新监控检查失败：", e.Error()))
				} else {
					ctx.SendChain(message.Text("检查完成；无实质变化不推送更新。可用“更新监控状态”查看来源覆盖。"))
				}
			}()
			return
		}
		if !mu.TryLock() {
			ctx.SendChain(message.Text("正在检查来源或模型，请稍后查看/修改配置。"))
			return
		}
		defer mu.Unlock()
		c, e := readConfig()
		if e != nil {
			ctx.SendChain(message.Text("配置读取失败：", e.Error()))
			return
		}
		switch {
		case arg == "状态" || arg == "":
			s, e := core.LoadState(statePath)
			if e != nil {
				ctx.SendChain(message.Text("状态读取失败：", e.Error()))
				return
			}
			last := "尚未检查"
			if !s.LastCheck.IsZero() {
				last = s.LastCheck.Local().Format("2006-01-02 15:04:05")
			}
			errors, _ := json.Marshal(s.SourceErrors)
			ctx.SendChain(message.Text(fmt.Sprintf("更新监控：%t\n间隔：%d小时\n每日时间（北京时间，空为间隔模式）：%s\n模型入口：%s\n最近检查：%s\n已建立基线来源：%d\n待通知：%d\n错误：%s\n来源失败：%s\nDiscord：未读取（仅支持配置官方公开转载）", c.Enabled, c.IntervalHours, c.DailyTime, c.ModelRole, last, len(s.Initialized), len(s.Pending), s.LastError, errors)))
			return
		case arg == "配置":
			b, _ := json.MarshalIndent(c, "", "  ")
			ctx.SendChain(message.Text(string(b)))
			return
		case arg == "测试通知":
			owner, err := resolveOwner(c)
			if err != nil {
				ctx.SendChain(message.Text("测试通知失败：", err))
				return
			}
			bot, _ := pickBot(c.BotID)
			if bot == nil {
				ctx.SendChain(message.Text("测试通知失败：未找到唯一在线机器人"))
				return
			}
			if !sendNotice(bot.CallAction, owner, "[更新监控测试通知] 私聊投递验收，不代表真实开发更新；未修改监控开关、计划、基线或待发队列。") {
				ctx.SendChain(message.Text("测试通知失败：QQ未确认投递成功"))
			}
			return
		case arg == "开启":
			c.Enabled = true
			if c.OwnerID == 0 {
				c.OwnerID = ctx.Event.UserID
			}
			if c.BotID == 0 {
				c.BotID = ctx.Event.SelfID
			}
		case arg == "暂停":
			c.Enabled = false
		case strings.HasPrefix(arg, "间隔 "):
			c.DailyTime = ""
			c.IntervalHours, e = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "间隔 ")))
		case strings.HasPrefix(arg, "模型 "):
			c.ModelRole = strings.TrimSpace(strings.TrimPrefix(arg, "模型 "))
			if c.ModelRole == "Chat" {
				c.ModelRole = ""
			}
		default:
			ctx.SendChain(message.Text("使用：更新监控状态/配置/开启/暂停/检查/间隔 24/模型 Summary"))
			return
		}
		if e == nil {
			e = c.Validate()
		}
		if e == nil {
			_, e = resolveOwner(c)
		}
		if e == nil {
			e = core.Save(configPath, c)
		}
		if e != nil {
			ctx.SendChain(message.Text("保存失败：", e.Error()))
			return
		}
		ctx.SendChain(message.Text("已保存，最迟1分钟生效。"))
	})
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if e := run(false); e != nil {
				log.Warn("[updatewatch] ", e)
			}
		}
	}()
}

func sendNotice(call func(string, zero.Params) zero.APIResponse, owner int64, text string) bool {
	response := call("send_private_msg", zero.Params{"user_id": owner, "message": message.Message{message.Text(text)}})
	return response.Status == "ok" && response.RetCode == 0 && response.Data.Get("message_id").Int() != 0
}
