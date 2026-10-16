package chat

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// AgentPersona is a complete optional override; absence means inherit global.
type AgentPersona struct {
	Character string `json:"character"`
	Sex       string `json:"sex"`
}

func personaKey(bot, conversation int64) string {
	return strconv.FormatInt(bot, 10) + ":" + strconv.FormatInt(conversation, 10)
}

func validPersonaKey(key string) bool {
	parts := strings.Split(key, ":")
	if len(parts) != 2 {
		return false
	}
	bot, e1 := strconv.ParseInt(parts[0], 10, 64)
	conversation, e2 := strconv.ParseInt(parts[1], 10, 64)
	return e1 == nil && e2 == nil && bot > 0 && conversation != 0 && conversation != -1<<63 && key == personaKey(bot, conversation)
}

func (p AgentPersona) Validate() error {
	if strings.TrimSpace(p.Character) == "" {
		return errors.New("自定义人物设定不能为空")
	}
	for name, value := range map[string]string{"AgentChar": p.Character, "AgentSex": p.Sex} {
		raw, _ := json.Marshal(value)
		if err := validateField(name, raw); err != nil {
			return err
		}
	}
	return nil
}

func (c AgentConfig) Persona(bot, conversation int64) (AgentPersona, bool) {
	if p, ok := c.Personas[personaKey(bot, conversation)]; ok {
		return p, true
	}
	return AgentPersona{Character: c.AgentChar, Sex: c.AgentSex}, false
}

// UpdatePersona uses the same atomic config revision and write-before backup as
// other settings, without invalidating agents already executing a task.
func (r *ConfigRepository) UpdatePersona(revision string, bot, conversation int64, persona *AgentPersona) (string, error) {
	c, _, err := r.Read()
	if err != nil {
		return "", err
	}
	if c.Personas == nil {
		c.Personas = map[string]AgentPersona{}
	}
	key := personaKey(bot, conversation)
	if !validPersonaKey(key) {
		return "", errors.New("会话人物范围无效")
	}
	if persona == nil {
		delete(c.Personas, key)
	} else {
		if err := persona.Validate(); err != nil {
			return "", err
		}
		c.Personas[key] = *persona
	}
	raw, _ := json.Marshal(c.Personas)
	return r.Update(revision, map[string]json.RawMessage{"Personas": raw})
}
