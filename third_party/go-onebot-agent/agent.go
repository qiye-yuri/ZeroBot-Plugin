// Package goba OneBot 11 协议 QQ 聊天 Agent
package goba

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/FloatTech/gg/factory"
	"github.com/FloatTech/ttl"
	"github.com/corona10/goimagehash"
	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/chat"
	"github.com/fumiama/deepinfra/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const (
	// EOA is a dummy action that is used to terminate request
	EOA = "end_action"
	// SVM is a dummy action that is used to indicate that a memory has been saved
	SVM = "save_memory"
	// SearchHistory is handled by the host as a scoped local read, never OneBot.
	SearchHistory = "search_history"
	ReadHistory   = "read_history"
	SearchMemory  = "search_memory"
	ReadMemory    = "read_memory"
)

var (
	// ErrPermissionDenied LLM 调用了不该调用的 action
	ErrPermissionDenied       = errors.New("permission denied")
	ErrCheckedHistoryRequired = errors.New("checked Agent preparation requires a history store")
)

// Agent is a OneBot event context, it is recommended to create one agent
// per group or per user.
type Agent struct {
	log          chat.Log[fmt.Stringer]
	history      HistoryStore
	historyPre   func(*fmt.Stringer)
	visionPre    func(*fmt.Stringer, func() error) error
	cfg          *Config
	perm         *Perm
	imgpcache    *ttl.Cache[uint64, imageDescription]
	mem          MemoryStorage
	manualaddreq bool
	manualaddmem bool
	hasimageapi  bool
}

// NewAgent 创建一个 Agent 实例。
//
//   - characteristics 推荐使用 Markdown 格式，描述 Agent 个性。
//   - defaultprompt 为上下文为空时的默认提示，建议为事件的 JSON，一般不会用到，因此也可留空。
//   - manualaddreq 表示是否由用户手动添加请求。
//   - manualaddmem 表示是否由用户手动添加记忆。
func NewAgent(
	cfg *Config, batchcap, itemscap int, imgpcachettl time.Duration,
	defaultprompt string, mem MemoryStorage,
	manualaddreq, manualaddmem bool,
) (ag Agent) {
	ag = Agent{
		log:          chat.NewLog[fmt.Stringer](batchcap, itemscap, "\n", defaultprompt),
		cfg:          cfg,
		imgpcache:    ttl.NewCache[uint64, imageDescription](imgpcachettl),
		mem:          mem,
		manualaddreq: manualaddreq, manualaddmem: manualaddmem,
	}
	_ = ag.LoadPermTable()
	return
}

// AddEvent 添加接收到的事件
func (ag *Agent) AddEvent(grp int64, ev *Event) error {
	return ag.record(grp, "event", ev, false)
}

// AddRequest 添加 API 请求, 一般无需主动调用, 由 GetAction 自动添加
func (ag *Agent) AddRequest(grp int64, req *zero.APIRequest) error {
	return ag.record(grp, "request", req, true)
}

// AddResponse 添加在执行完 zero.APIRequest 之后得到的响应
func (ag *Agent) AddResponse(grp int64, resp *APIResponse) error {
	return ag.record(grp, "response", resp, false)
}

// AddTerminus 添加会话终止符, 一般无需主动调用, 由 GetAction 自动添加
func (ag *Agent) AddTerminus(grp int64) error {
	return ag.record(grp, "end", Terminus{}, true)
}

// AddMemory 添加记忆, 一般无需主动调用, 由 GetAction 自动添加
func (ag *Agent) AddMemory(grp int64, text string) error {
	return ag.mem.Save(grp, strings.TrimSpace(text))
}

// SetMemoryStorage is configured before publishing an Agent to callers.
func (ag *Agent) SetMemoryStorage(mem MemoryStorage) { ag.mem = mem }

// CanViewImage will be true if SetViewImageAPI is called
func (ag *Agent) CanViewImage() bool {
	return ag.hasimageapi
}

// SetViewImageAPI 为 agent 增加识图功能, 需要模型支持视觉
func (ag *Agent) SetViewImageAPI(api deepinfra.API, p model.Protocol) {
	checkedPre := func(s *fmt.Stringer, check func() error) error {
		gate := func() error {
			if check != nil {
				return check()
			}
			return nil
		}
		o := *s
		if ev, ok := o.(*Event); ok {
			hasset := false
			msgs := message.ParseMessage(ev.Message)
			for i, msg := range msgs {
				if msg.Type != "image" {
					continue
				}
				if err := gate(); err != nil {
					return err
				}
				if text, ok := msg.Data["__agent_desc__"]; ok {
					desc, valid := normalizeImageDescription(text)
					if valid {
						desc.Truncated = desc.Truncated || msg.Data["__agent_desc_truncated__"] == "true"
						desc.apply(msgs[i].Data)
						hasset = true
						continue
					}
					delete(msgs[i].Data, "__agent_desc__")
					delete(msgs[i].Data, "__agent_desc_truncated__")
					hasset = true
				}
				u := msg.Data["url"]
				if u == "" {
					u = msg.Data["file"]
				}
				data, err := loadAgentImage(context.Background(), http.DefaultClient, u)
				if stopped := gate(); stopped != nil {
					return stopped
				}
				if err != nil {
					logrus.Debugln("[goba] SetViewImageAPI image input skipped:", err)
					continue
				}
				img, err := decodeAgentImage(data)
				if err != nil {
					logrus.Debugln("[goba] SetViewImageAPI decode img err:", err)
					continue
				}
				hsh, err := goimagehash.PerceptionHash(img)
				if err != nil {
					logrus.Debugln("[goba] SetViewImageAPI calc phash err:", err)
					continue
				}
				d := math.MaxInt
				desc := imageDescription{}
				_ = ag.imgpcache.Range(func(u uint64, s imageDescription) error {
					ds, err := goimagehash.NewImageHash(u, goimagehash.PHash).Distance(hsh)
					if err != nil {
						logrus.Debugln("[goba] SetViewImageAPI calc phash distance err:", err)
						return nil
					}
					if d > ds {
						d = ds
						desc = s
					}
					return nil
				})
				logrus.Debugln("[goba] SetViewImageAPI calculated min d:", d)
				if d < 8 { // very similar (>87.5%)
					desc.apply(msgs[i].Data)
					hasset = true
					logrus.Debugln("[goba] SetViewImageAPI hit cache.")
					continue
				}

				img = factory.Limit(img, 1024, 1024)
				data, err = factory.ToBytes(img)
				if err != nil {
					logrus.Debugln("[goba] SetViewImageAPI dump img err:", err)
					continue
				}
				imgs, err := model.NewContentImageDataBase64URL(data)
				if err != nil {
					logrus.Debugln("[goba] SetViewImageAPI conv b64 err:", err)
					continue
				}
				if err := gate(); err != nil {
					return err
				}
				visionProtocol := p.Clone() // Request-local protocol, never mutate the shared template.
				m := visionProtocol.User(
					model.NewContentImageURL(imgs),
					model.NewContentText("使用简洁清晰明确的一段中文纯文本描述图片"),
				)
				text, err := api.Request(&agentResponseModel{Model: m})
				if stopped := gate(); stopped != nil {
					return stopped
				}
				if err != nil {
					logrus.Debugln("[goba] SetViewImageAPI vision request failed")
					continue
				}
				var valid bool
				desc, valid = normalizeImageDescription(text)
				if !valid {
					logrus.Debugln("[goba] SetViewImageAPI empty or invalid description skipped")
					continue
				}
				desc.apply(msgs[i].Data)
				ag.imgpcache.Set(hsh.GetHash(), desc)
				hasset = true
			}
			if hasset {
				raw, err := json.Marshal(&msgs)
				if err == nil {
					ev.Message = raw
				}
			}
		}
		return nil
	}
	pre := func(s *fmt.Stringer) { _ = checkedPre(s, nil) }
	ag.log.SetPreModelize(pre)
	ag.historyPre = pre
	ag.visionPre = checkedPre
	ag.hasimageapi = true
}

// ClearViewImageAPI ...
func (ag *Agent) ClearViewImageAPI() {
	ag.log.SetPreModelize(nil)
	ag.historyPre = nil
	ag.visionPre = nil
	ag.hasimageapi = false
}

// GetAction get OneBot CallAction from LLM and add it to context.
//
// Note:
//
//   - Model API errors return ErrModelRequestFailed with safe default text and
//     an unwrap-able cause for errors.Is/As; task checks still take precedence.
//   - Invalid JSON object streams return ErrActionOutputInvalid with no requests
//     and no automatic writes from that batch. Size/count limits behave likewise.
//   - If LLM returns an invalid action, ErrPermissionDenied will be returned
//     with complete reqs before invalid call, caller may decide whether to use
//     these reqs by themselves. Whatever, invalid req will not be added into
//     the context. You may call AddRequest to add it but it is not recommended.
func (ag *Agent) GetAction(api deepinfra.API, p model.Protocol, grp int64, role PermRole, iter int, isusersystem bool) (
	reqs []zero.APIRequest, err error,
) {
	return ag.GetActionChecked(api, p, grp, role, iter, isusersystem, nil)
}

// GetActionChecked checks host task state between preparation operations and
// before/after the Agent request. It does not interrupt an admitted HTTP call.
// A non-nil check requires a HistoryStore; the legacy log has a shared callback.
func (ag *Agent) GetActionChecked(api deepinfra.API, p model.Protocol, grp int64, role PermRole, iter int, isusersystem bool, check func() error, characters ...Config) (
	reqs []zero.APIRequest, err error,
) {
	var preparationErr error
	gate := func() error {
		if preparationErr == nil && check != nil {
			preparationErr = check()
		}
		return preparationErr
	}
	if err = gate(); err != nil {
		return
	}
	if check != nil && ag.history == nil {
		return nil, ErrCheckedHistoryRequired
	}
	p = p.Clone() // Do not retain a prior response when callers reuse a protocol.
	sysp, err := ag.system(role, iter, grp, characters...)
	if err != nil {
		logrus.Debugln("[goba] GetAction get sysp err:", err)
		return
	}
	if err = gate(); err != nil {
		return
	}

	var m deepinfra.Model
	if ag.history != nil {
		pre := ag.historyPre
		if check != nil {
			pre = func(s *fmt.Stringer) {
				if gate() != nil {
					return
				}
				if ag.visionPre != nil {
					preparationErr = ag.visionPre(s, gate)
				} else if ag.historyPre != nil {
					ag.historyPre(s)
				}
			}
		}
		m, err = ag.history.Modelize(p, grp, sysp, isusersystem, pre)
		if preparationErr != nil {
			return nil, preparationErr
		}
		if err != nil {
			return
		}
	} else {
		m = ag.log.Modelize(p, grp, sysp, isusersystem)
	}
	if err = gate(); err != nil {
		return
	}

	resp, err := api.Request(&agentResponseModel{Model: m})
	if stopped := gate(); stopped != nil {
		return nil, stopped
	}
	if err != nil {
		logrus.Debugln("[goba] GetAction model request failed")
		if errors.Is(err, ErrActionOutputLimit) {
			return nil, ErrActionOutputLimit
		}
		return nil, &modelRequestError{cause: err}
	}
	if len(resp) > agentActionOutputBytes {
		return nil, ErrActionOutputLimit
	}
	if strings.HasPrefix(resp, "```") { // AI returns codeblock
		_, resp, _ = strings.Cut(resp, "\n")
		resp = strings.Trim(resp, "`")
		resp = strings.TrimSpace(resp)
	}
	if err = checkActionObjects(resp); err != nil {
		return nil, err
	}
	reqs = make([]zero.APIRequest, 0, 2)
	dec := json.NewDecoder(strings.NewReader(resp))
	dec.UseNumber()
	for dec.More() {
		r := zero.APIRequest{}
		err = dec.Decode(&r)
		if err != nil {
			logrus.Debugln("[goba] GetAction decode api request err:", err)
			break
		}
		if r.Action == "" {
			continue
		}
		switch {
		case r.Action == EOA:
			if err = ag.AddTerminus(grp); err != nil {
				return
			}
			err = io.EOF
			return
		case !ag.perm.allow(role, r.Action):
			err = errors.Wrap(ErrPermissionDenied, r.Action)
			return
		default:
			autoaddmem := !ag.manualaddmem && r.Action == SVM
			if !ag.manualaddreq || autoaddmem {
				if err = ag.AddRequest(grp, &r); err != nil {
					return
				}
			}
			if autoaddmem {
				txt, err := extractMemory(&r)
				if err != nil {
					logrus.Debugln("[goba] GetAction extract memory err:", err)
					ag.AddResponse(grp, &APIResponse{
						Status:  "error",
						Data:    []byte(`null`),
						Message: err.Error(),
					})
					continue
				}
				logrus.Debugln("[goba] GetAction add memory:", txt, "to grp:", grp)
				err = ag.AddMemory(grp, txt)
				s := "ok"
				msg := ""
				if err != nil {
					logrus.Debugln("[goba] GetAction add memory err:", err)
					s = "error"
					msg = err.Error()
				}
				logrus.Debugln("[goba] GetAction add memory response:", s, "msg:", msg)
				ag.AddResponse(grp, &APIResponse{
					Status:  s,
					Data:    []byte(`null`),
					Message: msg,
				})
			}
		}
		reqs = append(reqs, r)
	}

	return
}
