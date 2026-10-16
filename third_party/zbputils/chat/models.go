package chat

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/FloatTech/zbputils/modelconfig"
)

type ModelProfile struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Type  ModelType `json:"type"`
	Model string    `json:"model"`
	API   string    `json:"api"`
	Key   ModelKey  `json:"key"`
}

type ModelRoutes = modelconfig.Routes

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
	_, err := c.selectModels()
	return err
}

func (c AgentConfig) selectModels() (map[string]modelconfig.Profile, error) {
	profiles := make([]modelconfig.Profile, 0, len(c.ModelProfiles))
	for _, p := range c.ModelProfiles {
		profiles = append(profiles, modelconfig.Profile{ID: p.ID, Type: int(p.Type), API: p.API, Model: p.Model, Key: string(p.Key)})
	}
	return modelconfig.Select(profiles, *c.ModelRoutes)
}

// ResolveModels projects a single immutable selection into the existing clients.
// Empty optional routes disable that entrance, never fall back to another model.
func (c AgentConfig) ResolveModels() (AgentConfig, error) {
	if c.ModelRoutes == nil {
		if err := c.validateModelRoutes(); err != nil {
			return AgentConfig{}, err
		}
		return c, nil
	}
	profiles, err := c.selectModels()
	if err != nil {
		return AgentConfig{}, err
	}
	p := profiles[""]
	c.Type, c.ModelName, c.API, c.Key = ModelType(p.Type), p.Model, p.API, ModelKey(p.Key)
	p = profiles["Agent"]
	c.AgentType, c.AgentModelName, c.AgentAPI, c.AgentKey = ModelType(p.Type), p.Model, p.API, ModelKey(p.Key)
	p = profiles["Image"]
	c.ImageType, c.ImageModelName, c.ImageAPI, c.ImageKey = ModelType(p.Type), p.Model, p.API, ModelKey(p.Key)
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
