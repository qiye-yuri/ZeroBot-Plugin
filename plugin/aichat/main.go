// Package aichat 大模型聊天和Agent
package aichat

import (
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/RomiChan/syncx"
	"github.com/fumiama/deepinfra"
	goba "github.com/fumiama/go-onebot-agent"
	"github.com/sirupsen/logrus"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"

	"github.com/FloatTech/AnimeAPI/airecord"
	"github.com/FloatTech/ZeroBot-Plugin/internal/modelrequest"
	"github.com/FloatTech/ZeroBot-Plugin/internal/modelstats"
	"github.com/FloatTech/ZeroBot-Plugin/internal/qqvoice"
	"github.com/FloatTech/ZeroBot-Plugin/internal/replystats"
	"github.com/FloatTech/floatbox/process"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/chat"
	"github.com/FloatTech/zbputils/control"
)

var (
	// en data [8 temp] [8 rate] LSB
	en = control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Extra:            control.ExtraFromString("aichat"),
		Brief:            "大模型聊天和Agent",
		Help:             "- (随意聊天, 概率匹配)",

		PrivateDataFolder: "aichat",
	})
)

// Admission is atomic and keyed by both account and conversation. It also covers
// ordinary-chat fallback while an Agent task is winding down.
var conversationRuns sync.Map

var (
	fastfailnorecord atomic.Bool
)

func init() {
	en.OnMessage(chat.EnsureConfig, func(ctx *zero.Ctx) bool {
		if !chat.ConfigOf(ctx).Ready() {
			return false
		}
		stor, ok := ctx.State[zero.StateKeyPrefixKeep+"aichatcfg_stor__"].(chat.Storage)
		if !ok {
			logrus.Warnln("ERROR: cannot get stor")
			return false
		}
		mp := ctx.State[control.StateKeySyncxState].(*syncx.Map[string, any])
		if _, ok := mp.Load(chat.StateKeyAgentHooked); !ok && !stor.NoAgent() {
			logrus.Infoln("[aichat] skip agent for ctx has not been hooked by agent")
			return false
		}
		if !(ctx.ExtractPlainText() != "" &&
			(!stor.NoReplyAt() || (stor.NoReplyAt() && !ctx.Event.IsToMe))) {
			return false
		}
		rate := stor.Rate()
		if !ctx.Event.IsToMe && rand.Intn(100) >= int(rate) {
			return false
		}
		if ctx.Event.IsToMe {
			ctx.Block()
		}
		return true
	}).SetBlock(false).Handle(func(ctx *zero.Ctx) {
		conversation := ctx.Event.GroupID
		if conversation == 0 {
			conversation = -ctx.Event.UserID
		}
		key := [2]int64{ctx.Event.SelfID, conversation}
		if _, busy := conversationRuns.LoadOrStore(key, struct{}{}); busy {
			if ctx.Event.IsToMe {
				ctx.SendChain(message.Text("当前机器人会话正在处理上一条请求，请稍后再试。"))
			}
			return
		}
		defer conversationRuns.Delete(key)
		cfg := chat.ConfigOf(ctx)
		kind := replystats.Chat
		if !ctx.State[zero.StateKeyPrefixKeep+"aichatcfg_stor__"].(chat.Storage).NoAgent() && cfg.AgentAPI != "" && cfg.AgentModelName != "" && (cfg.AgentKey != "" || cfg.AgentType == 1) {
			kind = replystats.Agent
		}
		task := replystats.Default.Begin(ctx, kind)
		defer task.End()
		var activeRun *chat.AgentTask
		var runError error
		generation := chat.AgentGeneration()
		defer func() {
			p := recover()
			if p != nil {
				runError = errors.New("任务异常退出")
			}
			if activeRun != nil {
				if activeRun.Finish(runError) != nil {
					task.Fail()
				}
			}
			if p != nil {
				panic(p)
			}
		}()
		gid := ctx.Event.GroupID
		if gid == 0 {
			gid = -ctx.Event.UserID
		}
		stor := ctx.State[zero.StateKeyPrefixKeep+"aichatcfg_stor__"].(chat.Storage)
		temperature := stor.Temp()
		topp, maxn := cfg.MParams()
		mp := ctx.State[control.StateKeySyncxState].(*syncx.Map[string, any])

		logrus.Debugln("[aichat] agent mode test: noagent", stor.NoAgent(), "hasapi", cfg.AgentAPI != "", "hasmodel", cfg.AgentModelName != "")
		if !stor.NoAgent() && cfg.AgentAPI != "" && cfg.AgentModelName != "" && (cfg.AgentKey != "" || cfg.AgentType == 1) {
			store, taskErr := chat.AgentTasks(ctx.Event.SelfID)
			if taskErr != nil {
				task.Fail()
				return
			}
			sourceSeq := chat.AgentEventSequence(ctx)
			run, taskErr := store.BeginFrom(gid, sourceSeq)
			if taskErr != nil {
				task.Fail()
				return
			}
			activeRun = run
			runError = errors.New("任务提前结束")
			logrus.Debugln("[aichat] enter agent mode")
			x := deepinfra.NewAPI(cfg.AgentAPI, string(cfg.AgentKey))
			x.SetHTTPClient(modelstats.Default.Client(boundedAgentClient(&http.Client{Timeout: 30 * time.Second}), modelstats.Agent))
			mod, err := cfg.AgentType.ProtocolForAPI(cfg.AgentAPI, cfg.AgentModelName, temperature, topp, maxn, cfg.ReasoningEffort, cfg.Separator)
			if err != nil {
				task.Fail()
				logrus.Warnln("ERROR: ", err)
				return
			}
			role := agentRole(ctx)
			c, ok := ctx.State["manager"].(*ctrl.Control[*zero.Ctx])
			if !ok {
				logrus.Warnln("ERROR: cannot get ctrl mamager")
			}
			ag := chat.AgentOf(ctx.Event.SelfID, c.Service)
			logrus.Debugln("[aichat] got agent")
			if cfg.ImageAPI != "" && !ag.CanViewImage() {
				mod, err := cfg.ImageType.ProtocolForAPI(cfg.ImageAPI, cfg.ImageModelName, temperature, topp, maxn, cfg.ReasoningEffort, cfg.Separator)
				if err != nil {
					task.Fail()
					logrus.Warnln("ERROR: ", err)
					return
				}
				imageAPI := deepinfra.NewAPI(cfg.ImageAPI, string(cfg.ImageKey))
				imageAPI.SetHTTPClient(modelstats.Default.Client(boundedVisionClient(&http.Client{Timeout: 30 * time.Second}), modelstats.Vision))
				ag.SetViewImageAPI(imageAPI, mod)
				logrus.Debugln("[aichat] agent set img")
			}
			ctx.NoTimeout()
			logrus.Debugln("[aichat] agent set no timeout")
			hasresp := false
			search, err := chat.NewTaskHistorySearch(ctx.Event.SelfID, gid, run)
			if err != nil {
				runError = err
				task.Fail()
				return
			}
			err = chat.RunAgent(func(iter int) ([]zero.APIRequest, bool, error) {
				if e := run.Model(); e != nil {
					return nil, false, e
				}
				reqs, ended, e := chat.NextAgentActionsChecked(ag, zero.SuperUserPermission(ctx), iter, x, mod.Clone(), gid, role, func() error {
					if generation != chat.AgentGeneration() {
						return chat.ErrAgentReloaded
					}
					return run.Check()
				})
				if errors.Is(e, errAgentResponseLimit) {
					e = goba.ErrActionOutputLimit
				}
				if stopped := run.Check(); stopped != nil {
					return nil, false, stopped
				}
				return reqs, ended, e
			}, func(req zero.APIRequest) (actionErr error) {
				if e := run.Action(req.Action); e != nil {
					return e
				}
				outcome := "local_error"
				defer func() {
					if e := run.Result(outcome); e != nil {
						actionErr = e
					}
				}()
				mp.Store(chat.StateKeyAgentTriggered, struct{}{})
				if req.Action == goba.SearchHistory || req.Action == goba.ReadHistory || req.Action == goba.SearchMemory || req.Action == goba.ReadMemory {
					e := search.Execute(ag, req)
					outcome = search.Outcome()
					return e
				}
				if req.Action == goba.SVM {
					if err := ag.AddRequest(gid, &req); err != nil {
						return err
					}
					text, ok := req.Params["text"].(string)
					if !ok || len(req.Params) != 1 {
						return chat.ErrMemoryInput
					}
					store, err := chat.AgentMemories(ctx.Event.SelfID)
					if err != nil {
						return err
					}
					saved, err := store.SaveFromSequenceResult(gid, text, sourceSeq)
					if err != nil {
						return err
					}
					data, err := json.Marshal(saved)
					if err != nil {
						return err
					}
					if err = ag.AddResponse(gid, &goba.APIResponse{Status: "ok", Data: data}); err != nil {
						return err
					}
					outcome = "ok"
					return nil
				}
				logrus.Debugln("[chat] agent triggered", gid, "add requ:", &req)
				if err := ag.AddRequest(gid, &req); err != nil {
					return err
				}
				hasresp = true
				outcome = "unknown"
				rsp := task.Call(req.Action, req.Params, ctx.CallAction)
				outcome = "unknown"
				if rsp.Status == "ok" && rsp.RetCode == 0 {
					outcome = "ok"
				} else if rsp.Status == "failed" {
					outcome = "rejected"
				}
				if rsp.Status != "ok" || rsp.RetCode != 0 {
					task.Fail()
				}
				logrus.Debugln("[chat] agent triggered", gid, "add resp:", &rsp)
				if e := ag.AddResponse(gid, &goba.APIResponse{
					Status:  rsp.Status,
					Data:    json.RawMessage(rsp.Data.Raw),
					Message: rsp.Message,
					Wording: rsp.Wording,
					RetCode: rsp.RetCode,
				}); e != nil {
					return e
				}
				if outcome != "ok" {
					return errors.New("平台动作未成功或结果未知")
				}
				return nil
			}, func() bool { return generation == chat.AgentGeneration() })
			if stopped := run.Check(); stopped != nil {
				err = stopped
			}
			runError = err
			if err != nil {
				task.Fail()
				logrus.Warnln("[aichat] Agent 本次任务已停止，检查结束动作、记录存储或重复动作。")
				return
			}
			if hasresp {
				return
			}
			// no response, fall back to normal chat
			logrus.Debugln("[aichat] agent fell back to normal chat")
		}
		checkRun := func() error {
			if activeRun == nil {
				return nil
			}
			if generation != chat.AgentGeneration() {
				return chat.ErrAgentReloaded
			}
			return activeRun.Check()
		}
		fallbackCall := func(action string, params zero.Params) zero.APIResponse {
			if activeRun != nil {
				if runError != nil {
					return zero.APIResponse{}
				}
				if runError = checkRun(); runError != nil {
					return zero.APIResponse{}
				}
				if runError = activeRun.Action(action); runError != nil {
					return zero.APIResponse{}
				}
			}
			rsp := task.Call(action, params, ctx.CallAction)
			if activeRun != nil {
				outcome := "unknown"
				if rsp.Status == "ok" && rsp.RetCode == 0 {
					outcome = "ok"
				} else if rsp.Status == "failed" {
					outcome = "rejected"
				}
				runError = activeRun.Result(outcome)
				if runError == nil && outcome != "ok" {
					runError = errors.New("回复结果未成功或未知")
				}
			}
			return rsp
		}
		if activeRun != nil {
			if runError = checkRun(); runError != nil {
				return
			}
			if runError = activeRun.FallbackModel(); runError != nil {
				return
			}
			runError = errors.New("聊天兜底提前结束")
		}

		x := deepinfra.NewAPI(cfg.API, string(cfg.Key))
		x.SetHTTPClient(modelstats.Default.Client(&http.Client{Timeout: 30 * time.Second}, modelstats.Chat))
		mod, err := cfg.Type.ProtocolForAPI(cfg.API, cfg.ModelName, temperature, topp, maxn, cfg.ReasoningEffort, cfg.Separator)
		if err != nil {
			task.Fail()
			logrus.Warnln("ERROR: ", err)
			return
		}
		data, err := modelrequest.Request(x, chat.GetChatContext(mod, gid, cfg.SystemP, bool(cfg.NoSystemP)))
		if err != nil {
			task.Fail()
			logrus.Warnln("[aichat] 模型请求失败，请检查接口、密钥及网络。")
			return
		}
		if activeRun != nil {
			runError = checkRun()
			if runError != nil {
				return
			}
		}

		txt := chat.Sanitize(strings.Trim(data, "\n 　"))
		if len(txt) > 0 {
			chat.AddChatReply(gid, txt)
			nick := zero.BotConfig.NickName[rand.Intn(len(zero.BotConfig.NickName))]
			txt = strings.ReplaceAll(txt, "{name}", ctx.CardOrNickName(ctx.Event.UserID))
			txt = strings.ReplaceAll(txt, "{me}", nick)
			id := any(nil)
			if ctx.Event.IsToMe {
				id = ctx.Event.MessageID
			}
			for _, t := range strings.Split(txt, "{segment}") {
				if t == "" {
					continue
				}
				if activeRun != nil {
					runError = checkRun()
					if runError != nil {
						return
					}
				}
				logrus.Debugln("[aichat] 回复内容:", t)
				recCfg := airecord.GetConfig()
				if ctx.Event.GroupID > 0 && !fastfailnorecord.Load() && !stor.NoRecord() {
					if qqvoice.Deliver(t, recCfg.ModelID, ctx.Event.GroupID, func(action string, params zero.Params) zero.APIResponse {
						return fallbackCall(action, params)
					}) == "" {
						continue
					}
					if activeRun != nil && runError != nil {
						return
					}
					fastfailnorecord.Store(true)
				}
				if activeRun != nil {
					msg := message.Message{message.Text(t)}
					if id != nil {
						msg = append(message.Message{message.Reply(id)}, msg...)
					}
					action := "send_private_msg"
					params := zero.Params{"user_id": ctx.Event.UserID, "message": msg}
					if ctx.Event.GroupID != 0 {
						action = "send_group_msg"
						params = zero.Params{"group_id": ctx.Event.GroupID, "message": msg}
					}
					rsp := fallbackCall(action, params)
					if runError != nil {
						return
					}
					id = message.NewMessageIDFromInteger(rsp.Data.Get("message_id").Int())
				} else if id != nil {
					id = task.SendChain(ctx, message.Reply(id), message.Text(t))
				} else {
					id = task.SendChain(ctx, message.Text(t))
				}
				process.SleepAbout1sTo2s()
			}
		}
	})
}
