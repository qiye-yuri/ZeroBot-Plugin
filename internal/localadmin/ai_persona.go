package localadmin

import (
	"net/http"
	"strings"

	"github.com/FloatTech/zbputils/chat"
)

type personaRequest struct {
	BotID     string `json:"botId"`
	Type      string `json:"type"`
	ID        string `json:"id"`
	Revision  string `json:"revision"`
	Mode      string `json:"mode"`
	Character string `json:"character"`
	Sex       string `json:"sex"`
}

func (s *Server) aiPersona(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AI == nil || s.cfg.AI.repository == nil {
		apiError(w, 409, "当前程序未接入 AI 设置")
		return
	}
	var req personaRequest
	write := r.Method == http.MethodPost
	if write {
		if !readJSONLimit(w, r, &req, 131072) {
			return
		}
	} else {
		q := r.URL.Query()
		req = personaRequest{BotID: q.Get("botId"), Type: q.Get("type"), ID: q.Get("id")}
	}
	bot, ok := archiveID(req.BotID)
	id, valid := archiveID(req.ID)
	if !ok || !valid || (req.Type != "group" && req.Type != "private") {
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
	if write {
		if !validRevision(req.Revision) {
			apiError(w, 400, "配置版本无效，请重新读取")
			return
		}
		var persona *chat.AgentPersona
		switch req.Mode {
		case "custom":
			persona = &chat.AgentPersona{Character: strings.TrimSpace(req.Character), Sex: req.Sex}
			if err := persona.Validate(); err != nil {
				apiError(w, 400, err.Error())
				return
			}
		case "inherit":
		default:
			apiError(w, 400, "人物模式须为继承全局或自定义")
			return
		}
		if _, err := s.cfg.AI.repository.UpdatePersona(req.Revision, bot, id, persona); err != nil {
			s.aiSaveError(w, err)
			return
		}
	}
	c, revision, err := s.cfg.AI.repository.Read()
	if err != nil {
		apiError(w, 500, "人物设置读取失败，请重新加载")
		return
	}
	persona, custom := c.Persona(bot, id)
	mode := "inherit"
	if custom {
		mode = "custom"
	}
	jsonReply(w, 200, map[string]any{"botId": req.BotID, "type": req.Type, "id": req.ID, "revision": revision, "mode": mode, "character": c.Redact(persona.Character), "sex": c.Redact(persona.Sex)})
}
