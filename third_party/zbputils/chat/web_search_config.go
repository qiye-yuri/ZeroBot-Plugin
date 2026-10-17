package chat

import (
	"encoding/json"
	"errors"

	goba "github.com/fumiama/go-onebot-agent"
)

type WebSearchPolicy struct {
	Enabled bool     `json:"enabled"`
	Roles   []string `json:"roles"`
}

type WebSearchConfig struct {
	Enabled bool                       `json:"enabled"`
	Key     ModelKey                   `json:"key"`
	Scopes  map[string]WebSearchPolicy `json:"scopes"`
}

func WebSearchScope(bot, conversation int64) string { return personaKey(bot, conversation) }

func (c WebSearchConfig) Validate() error {
	key, _ := json.Marshal(c.Key)
	if err := validateField("Key", key); err != nil {
		return err
	}
	if c.Enabled && c.Key == "" {
		return errors.New("启用搜索前请配置 DeepSeek 官方 API Key")
	}
	for scope, p := range c.Scopes {
		if !validPersonaKey(scope) {
			return errors.New("搜索会话范围无效")
		}
		seen := map[string]bool{}
		for _, role := range p.Roles {
			if (role != "owner" && role != "admin" && role != "user") || seen[role] {
				return errors.New("搜索角色须为主人、群管理或普通成员，且不能重复")
			}
			seen[role] = true
		}
		if p.Enabled && len(p.Roles) == 0 {
			return errors.New("启用会话搜索须选择允许角色")
		}
	}
	return nil
}

func (c AgentConfig) SearchPolicy(bot, conversation int64) WebSearchPolicy {
	if c.WebSearch != nil {
		if p, ok := c.WebSearch.Scopes[personaKey(bot, conversation)]; ok {
			if p.Roles == nil {
				p.Roles = []string{}
			}
			return p
		}
	}
	return WebSearchPolicy{Roles: []string{"owner"}}
}

func (c AgentConfig) SearchAllowed(bot, conversation int64, role goba.PermRole) bool {
	if c.WebSearch == nil || !c.WebSearch.Enabled || c.WebSearch.Key == "" {
		return false
	}
	p := c.SearchPolicy(bot, conversation)
	if !p.Enabled {
		return false
	}
	for _, allowed := range p.Roles {
		if allowed == string(role) {
			return true
		}
	}
	return false
}

func (r *ConfigRepository) UpdateWebSearch(revision string, config WebSearchConfig) (string, error) {
	raw, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return r.Update(revision, map[string]json.RawMessage{"WebSearch": raw})
}
