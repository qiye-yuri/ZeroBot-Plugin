package chat

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

type ModelProfile struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Type  ModelType `json:"type"`
	Model string    `json:"model"`
	API   string    `json:"api"`
	Key   ModelKey  `json:"key"`
}

type ModelRoutes struct {
	Chat  string `json:"chat"`
	Agent string `json:"agent"`
	Image string `json:"image"`
}

var profileID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var ErrNamedModelFields = errors.New("已启用命名模型，请在后台命名模型配置中修改模型、接口和密钥")
var ErrInvalidModelConfig = errors.New("命名模型配置无效")

func (p ModelProfile) Validate() error {
	if !profileID.MatchString(p.ID) || strings.TrimSpace(p.Name) == "" {
		return errors.New("模型配置需要有效编号和名称")
	}
	for field, value := range map[string]any{"AgentSex": p.Name, "Type": p.Type, "ModelName": p.Model, "API": p.API, "Key": p.Key} {
		raw, err := json.Marshal(value)
		if err != nil {
			return errors.New("模型配置格式无效")
		}
		if err = validateField(field, raw); err != nil {
			return err
		}
	}
	return nil
}

func (c AgentConfig) validateModelRoutes() error {
	if c.ModelRoutes == nil {
		if c.ModelProfiles != nil {
			return errors.New("命名模型缺少入口选择")
		}
		return nil
	}
	ids := map[string]ModelProfile{}
	for _, p := range c.ModelProfiles {
		if _, ok := ids[p.ID]; ok {
			return errors.New("模型配置编号重复")
		}
		ids[p.ID] = p
	}
	if c.ModelRoutes.Chat == "" {
		return errors.New("聊天入口必须选择模型配置")
	}
	for _, id := range []string{c.ModelRoutes.Chat, c.ModelRoutes.Agent, c.ModelRoutes.Image} {
		if id == "" {
			continue
		}
		p, ok := ids[id]
		if !ok {
			return errors.New("入口引用的命名模型不存在，请重新选择")
		}
		if p.API == "" || p.Model == "" || (p.Type != 1 && p.Key == "") {
			return errors.New("入口引用的命名模型配置不完整，请核对地址、模型与密钥")
		}
	}
	return nil
}

// ResolveModels projects a single immutable selection into the existing clients.
// Empty optional routes disable that entrance, never fall back to another model.
func (c AgentConfig) ResolveModels() (AgentConfig, error) {
	if err := c.validateModelRoutes(); err != nil {
		return AgentConfig{}, err
	}
	if c.ModelRoutes == nil {
		return c, nil
	}
	profiles := map[string]ModelProfile{}
	for _, p := range c.ModelProfiles {
		profiles[p.ID] = p
	}
	p := profiles[c.ModelRoutes.Chat]
	c.Type, c.ModelName, c.API, c.Key = p.Type, p.Model, p.API, p.Key
	p = profiles[c.ModelRoutes.Agent]
	c.AgentType, c.AgentModelName, c.AgentAPI, c.AgentKey = p.Type, p.Model, p.API, p.Key
	p = profiles[c.ModelRoutes.Image]
	c.ImageType, c.ImageModelName, c.ImageAPI, c.ImageKey = p.Type, p.Model, p.API, p.Key
	return c, nil
}

// ModelCatalog returns an in-memory migration preview; reading never writes.
func (c AgentConfig) ModelCatalog() ([]ModelProfile, ModelRoutes) {
	if c.ModelRoutes != nil {
		return append([]ModelProfile(nil), c.ModelProfiles...), *c.ModelRoutes
	}
	profiles := []ModelProfile{}
	add := func(id, name string, kind ModelType, model, api string, key ModelKey) string {
		for _, p := range profiles {
			if p.Type == kind && p.Model == model && p.API == api && p.Key == key {
				return p.ID
			}
		}
		profiles = append(profiles, ModelProfile{ID: id, Name: name, Type: kind, Model: model, API: api, Key: key})
		return id
	}
	routes := ModelRoutes{Chat: add("legacy-chat", "原聊天模型", c.Type, c.ModelName, c.API, c.Key)}
	if c.AgentAPI != "" || c.AgentModelName != "" || c.AgentKey != "" {
		routes.Agent = add("legacy-agent", "原 Agent 模型", c.AgentType, c.AgentModelName, c.AgentAPI, c.AgentKey)
	}
	if c.ImageAPI != "" || c.ImageModelName != "" || c.ImageKey != "" {
		routes.Image = add("legacy-image", "原识图模型", c.ImageType, c.ImageModelName, c.ImageAPI, c.ImageKey)
	}
	return profiles, routes
}

func modelField(name string) bool {
	switch name {
	case "Type", "API", "Key", "ModelName", "AgentType", "AgentAPI", "AgentKey", "AgentModelName", "ImageType", "ImageAPI", "ImageKey", "ImageModelName":
		return true
	}
	return false
}

func (r *ConfigRepository) UpdateModels(revision string, profiles []ModelProfile, routes ModelRoutes) (string, error) {
	p, _ := json.Marshal(profiles)
	routesJSON, _ := json.Marshal(routes)
	return r.Update(revision, map[string]json.RawMessage{"ModelProfiles": p, "ModelRoutes": routesJSON})
}
