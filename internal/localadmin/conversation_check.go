package localadmin

import (
	"net/http"
	"time"

	"github.com/FloatTech/zbputils/chat"
	"github.com/FloatTech/zbputils/modelconfig"
)

type conversationModelCheck struct {
	Entry            string                        `json:"entry"`
	Applicable       bool                          `json:"applicable"`
	Enabled          bool                          `json:"enabled"`
	Inherited        bool                          `json:"inherited"`
	ProfileID        string                        `json:"profileId"`
	Name             string                        `json:"name"`
	Model            string                        `json:"model"`
	Type             chat.ModelType                `json:"type"`
	Check            AIConfigCheck                 `json:"check"`
	Parameters       modelconfig.RequestParameters `json:"parameters"`
	ParameterSources map[string]string             `json:"parameterSources"`
}

func conversationModels(c chat.AgentConfig, private bool) map[string]any {
	resolved, err := c.ResolveModels()
	if err != nil {
		// The same resolver used for requests rejects invalid references. Do not
		// construct a plausible selection from the legacy fields on this path.
		return map[string]any{"named": c.ModelRoutes != nil, "error": "模型入口配置无效，请在 AI 设置核对命名模型引用及必填字段", "entries": []conversationModelCheck{}}
	}
	profiles, routes := c.ModelCatalog()
	byID := map[string]chat.ModelProfile{}
	for _, p := range profiles {
		byID[p.ID] = p
	}
	ids := map[string]string{"chat": routes.Chat, "gpt": routes.GPT, "summary": routes.Summary, "agent": routes.Agent, "image": routes.Image}
	entries := []conversationModelCheck{}
	for _, item := range []struct{ entry, role string }{{"chat", ""}, {"gpt", "GPT"}, {"summary", "Summary"}, {"agent", "Agent"}, {"image", "Image"}} {
		id := ids[item.entry]
		inherited := (item.entry == "gpt" || item.entry == "summary") && id == ""
		if inherited {
			id = routes.Chat
		}
		p := byID[id]
		target := resolved.ForModel(item.role)
		params := resolved.RequestParams(item.role)
		params.ReasoningEffort = c.Redact(params.ReasoningEffort)
		sources := map[string]string{"maxN": "global", "topP": "global", "reasoningEffort": "global"}
		if c.ModelRoutes != nil && p.Parameters != nil {
			if p.Parameters.MaxN != nil {
				sources["maxN"] = "model"
			}
			if p.Parameters.TopP != nil {
				sources["topP"] = "model"
			}
			if p.Parameters.ReasoningEffort != nil {
				sources["reasoningEffort"] = "model"
			}
		}
		entries = append(entries, conversationModelCheck{
			Entry: item.entry, Applicable: !(private && item.entry == "summary"),
			Enabled: id != "", Inherited: inherited, ProfileID: c.Redact(id), Name: c.Redact(p.Name),
			Model: c.Redact(target.ModelName), Type: target.Type,
			Check:      checkAIConfig(target.Type, target.ModelName, target.API, string(target.Key), "API", item.entry != "image"),
			Parameters: params, ParameterSources: sources,
		})
	}
	return map[string]any{"named": c.ModelRoutes != nil, "error": "", "entries": entries}
}

func (s *Server) conversationCheck(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	botID, kind, conversationID := q.Get("botId"), q.Get("type"), q.Get("id")
	bot, botOK := archiveID(botID)
	id, idOK := archiveID(conversationID)
	if !botOK || !idOK || (kind != "group" && kind != "private") {
		apiError(w, 400, "请选择有效机器人及群/私聊范围")
		return
	}
	known := false
	for _, account := range s.cfg.Overview().BotAccounts {
		if account == botID {
			known = true
			break
		}
	}
	if !known {
		apiError(w, 404, "机器人当前未连接")
		return
	}
	if s.cfg.AI == nil || s.cfg.AI.repository == nil || s.cfg.PluginControl == nil || s.cfg.PluginControl.responseManager == nil {
		apiError(w, 409, "当前程序未接入 AI 或完整插件控制读取")
		return
	}
	if kind == "private" {
		id = -id
	}
	c, revision, err := s.cfg.AI.repository.Read()
	if err != nil {
		apiError(w, 500, "AI 配置读取失败，请重新读取")
		return
	}
	groups, err := s.cfg.AI.conversationSettings(id)
	if err != nil {
		apiError(w, 500, "会话参数读取失败，请重新读取")
		return
	}
	response, err := s.cfg.PluginControl.responseManager.ReadConversationResponseState(id)
	if err != nil {
		apiError(w, 500, "响应状态读取失败，请检查本机控制数据库")
		return
	}
	plugins := []map[string]any{}
	services := []string{"aichat", "llm"}
	if s.cfg.AI.unified {
		services = []string{"aichat"}
	}
	for _, service := range services {
		plugin := map[string]any{"service": service, "registered": false}
		if control := s.cfg.PluginControl.controls[service]; control != nil {
			state, err := control.SwitchState(id)
			if err != nil {
				apiError(w, 500, "插件状态读取失败，请检查本机控制数据库")
				return
			}
			plugin["registered"], plugin["state"] = true, state
		}
		plugins = append(plugins, plugin)
	}
	_, custom := c.Persona(bot, id)
	mode := "inherit"
	if custom {
		mode = "custom"
	}
	enabled, key := false, false
	if c.WebSearch != nil {
		enabled, key = c.WebSearch.Enabled, c.WebSearch.Key != ""
	}
	source := response.Source
	if source == "group" {
		source = "conversation"
	}
	jsonReply(w, 200, map[string]any{
		"scope":      map[string]string{"botId": botID, "type": kind, "id": conversationID},
		"observedAt": s.now().UTC().Format(time.RFC3339Nano), "revision": revision,
		"response": map[string]any{"globalEnabled": response.GlobalEnabled, "conversationEnabled": response.GroupEnabled, "effectiveEnabled": response.EffectiveEnabled, "source": source},
		"plugins":  plugins, "conversation": groups, "models": conversationModels(c, kind == "private"),
		"persona": map[string]string{"mode": mode},
		"search":  map[string]any{"enabled": enabled, "keyConfigured": key, "policy": c.SearchPolicy(bot, id)},
	})
}
