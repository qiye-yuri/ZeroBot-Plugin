package localadmin

import (
	"net/http"

	"github.com/FloatTech/zbputils/chat"
)

type searchSettingsRequest struct {
	BotID    string               `json:"botId"`
	Type     string               `json:"type"`
	ID       string               `json:"id"`
	Revision string               `json:"revision"`
	Enabled  bool                 `json:"enabled"`
	Key      AIKeyChange          `json:"key"`
	Policy   chat.WebSearchPolicy `json:"policy"`
}

func (s *Server) aiSearch(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AI == nil || s.cfg.AI.repository == nil {
		apiError(w, 409, "当前程序未接入 AI 设置")
		return
	}
	var req searchSettingsRequest
	write := r.Method == http.MethodPost
	if write {
		if !readJSONLimit(w, r, &req, 16384) {
			return
		}
	} else {
		q := r.URL.Query()
		req.BotID, req.Type, req.ID = q.Get("botId"), q.Get("type"), q.Get("id")
	}
	bot, bok := archiveID(req.BotID)
	id, iok := archiveID(req.ID)
	if !bok || !iok || (req.Type != "group" && req.Type != "private") {
		apiError(w, 400, "请选择有效机器人及群/私聊范围")
		return
	}
	known := false
	for _, account := range s.cfg.Overview().BotAccounts {
		if account == req.BotID {
			known = true
			break
		}
	}
	if !known {
		apiError(w, 404, "机器人当前未连接")
		return
	}
	if req.Type == "private" {
		id = -id
	}
	c, revision, err := s.cfg.AI.repository.Read()
	if err != nil {
		apiError(w, 500, "搜索设置读取失败")
		return
	}
	if write {
		if !validRevision(req.Revision) {
			apiError(w, 400, "请重新读取有效配置版本")
			return
		}
		config := chat.WebSearchConfig{}
		if c.WebSearch != nil {
			config = *c.WebSearch
		}
		switch req.Key.Action {
		case "keep":
			if req.Key.Value != "" || req.Key.Confirm {
				apiError(w, 400, "保留密钥无需新值或确认")
				return
			}
		case "replace":
			if req.Key.Value == "" || req.Key.Confirm {
				apiError(w, 400, "替换密钥须填写新值")
				return
			}
			config.Key = chat.ModelKey(req.Key.Value)
		case "clear":
			if !req.Key.Confirm || req.Key.Value != "" {
				apiError(w, 400, "请确认清除搜索密钥")
				return
			}
			config.Key = ""
		default:
			apiError(w, 400, "搜索密钥操作无效")
			return
		}
		config.Enabled = req.Enabled
		if config.Scopes == nil {
			config.Scopes = map[string]chat.WebSearchPolicy{}
		}
		// Scope uses the same exact bot/conversation key as persona overrides.
		config.Scopes[chat.WebSearchScope(bot, id)] = req.Policy
		if err = config.Validate(); err != nil {
			apiError(w, 400, err.Error())
			return
		}
		if s.cfg.Logs != nil {
			s.cfg.Logs.AddSecrets(req.Key.Value)
		}
		if _, err = s.cfg.AI.repository.UpdateWebSearch(req.Revision, config); err != nil {
			s.aiSaveError(w, err)
			return
		}
		c, revision, err = s.cfg.AI.repository.Read()
		if err != nil {
			apiError(w, 500, "搜索设置保存后读取失败")
			return
		}
	}
	enabled, keyConfigured := false, false
	if c.WebSearch != nil {
		enabled, keyConfigured = c.WebSearch.Enabled, c.WebSearch.Key != ""
	}
	jsonReply(w, 200, map[string]any{"botId": req.BotID, "type": req.Type, "id": req.ID, "revision": revision, "enabled": enabled, "keyConfigured": keyConfigured, "policy": c.SearchPolicy(bot, id)})
}
