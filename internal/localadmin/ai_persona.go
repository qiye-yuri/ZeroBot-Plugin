package localadmin

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

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
	Name      string `json:"name"`
	CardIndex *int   `json:"cardIndex"`
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
		var saveErr error
		switch req.Mode {
		case "saveCard", "applyCard":
			c, revision, err := s.cfg.AI.repository.Read()
			if err != nil {
				apiError(w, 500, "角色卡读取失败")
				return
			}
			if revision != req.Revision {
				apiError(w, 409, "角色设置已变化，请重新读取")
				return
			}
			if req.Mode == "applyCard" {
				if req.CardIndex == nil || *req.CardIndex < 0 || *req.CardIndex >= len(c.PersonaCards) || c.PersonaCards[*req.CardIndex].BotID != req.BotID {
					apiError(w, 400, "请选择当前机器人的角色卡")
					return
				}
				card := c.PersonaCards[*req.CardIndex]
				persona = &card.AgentPersona
			} else {
				name := strings.TrimSpace(req.Name)
				if name == "" || utf8.RuneCountInString(name) > 64 || strings.IndexFunc(name, unicode.IsControl) >= 0 || c.Redact(name) != name {
					apiError(w, 400, "请输入有效卡名，最多64字")
					return
				}
				for _, card := range c.PersonaCards {
					if card.BotID == req.BotID && card.Name == name {
						apiError(w, 409, "当前机器人已有同名卡，请换个卡名")
						return
					}
				}
				current, _ := c.Persona(bot, id)
				if err := current.Validate(); err != nil {
					apiError(w, 400, err.Error())
					return
				}
				cards, _ := json.Marshal(append(c.PersonaCards, chat.PersonaCard{BotID: req.BotID, Name: name, AgentPersona: current}))
				_, saveErr = s.cfg.AI.repository.Update(revision, map[string]json.RawMessage{"PersonaCards": cards})
			}
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
		if req.Mode != "saveCard" {
			_, saveErr = s.cfg.AI.repository.UpdatePersona(req.Revision, bot, id, persona)
		}
		if saveErr != nil {
			s.aiSaveError(w, saveErr)
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
	cards := []map[string]any{}
	for i, card := range c.PersonaCards {
		if card.BotID == req.BotID {
			cards = append(cards, map[string]any{"index": i, "name": c.Redact(card.Name), "character": c.Redact(card.Character), "sex": c.Redact(card.Sex)})
		}
	}
	jsonReply(w, 200, map[string]any{"botId": req.BotID, "type": req.Type, "id": req.ID, "revision": revision, "mode": mode, "character": c.Redact(persona.Character), "sex": c.Redact(persona.Sex), "cards": cards})
}
