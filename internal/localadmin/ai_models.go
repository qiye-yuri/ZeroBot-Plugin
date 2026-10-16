package localadmin

import (
	"errors"
	"net/http"

	"github.com/FloatTech/zbputils/chat"
	"github.com/FloatTech/zbputils/modelconfig"
)

type modelProfileView struct {
	ID                  string                        `json:"id"`
	Name                string                        `json:"name"`
	Type                chat.ModelType                `json:"type"`
	Model               string                        `json:"model"`
	API                 string                        `json:"api"`
	KeyConfigured       bool                          `json:"keyConfigured"`
	AddressRedacted     bool                          `json:"addressRedacted"`
	Parameters          *modelconfig.Parameters       `json:"parameters,omitempty"`
	EffectiveParameters modelconfig.RequestParameters `json:"effectiveParameters"`
}

type modelProfileUpdate struct {
	ID         string                  `json:"id"`
	Name       string                  `json:"name"`
	Type       chat.ModelType          `json:"type"`
	Model      string                  `json:"model"`
	API        string                  `json:"api"`
	Key        AIKeyChange             `json:"key"`
	Parameters *modelconfig.Parameters `json:"parameters,omitempty"`
}

type modelsUpdate struct {
	Revision string               `json:"revision"`
	Profiles []modelProfileUpdate `json:"profiles"`
	Routes   chat.ModelRoutes     `json:"routes"`
}

type modelRouteView struct {
	ProfileID  string                        `json:"profileId"`
	Inherited  bool                          `json:"inherited"`
	Parameters modelconfig.RequestParameters `json:"parameters"`
}

func modelRouteViews(profiles []chat.ModelProfile, routes chat.ModelRoutes, defaults modelconfig.RequestParameters) map[string]modelRouteView {
	byID := map[string]chat.ModelProfile{}
	for _, p := range profiles {
		byID[p.ID] = p
	}
	views := map[string]modelRouteView{}
	for entry, id := range map[string]string{"chat": routes.Chat, "gpt": routes.GPT, "summary": routes.Summary, "agent": routes.Agent, "image": routes.Image} {
		inherited := (entry == "gpt" || entry == "summary") && id == ""
		if inherited {
			id = routes.Chat
		}
		views[entry] = modelRouteView{ProfileID: id, Inherited: inherited, Parameters: byID[id].Parameters.Resolve(defaults)}
	}
	return views
}

func (s *AIStore) SaveModels(update modelsUpdate) error {
	if !validRevision(update.Revision) {
		return invalidAI("配置版本无效")
	}
	c, _, err := s.repository.Read()
	if err != nil {
		return err
	}
	old, _ := c.ModelCatalog()
	byID := map[string]chat.ModelProfile{}
	for _, p := range old {
		byID[p.ID] = p
	}
	profiles := make([]chat.ModelProfile, 0, len(update.Profiles))
	for _, item := range update.Profiles {
		previous, exists := byID[item.ID]
		p := chat.ModelProfile{ID: item.ID, Name: item.Name, Type: item.Type, Model: item.Model, API: item.API, Parameters: item.Parameters}
		if display, redacted := chat.DisplayAPI(previous.API); exists && redacted && item.API == display {
			p.API = previous.API
		}
		switch item.Key.Action {
		case "keep":
			if item.Key.Value != "" || item.Key.Confirm {
				return invalidAI("保留密钥无需新值或确认")
			}
			p.Key = previous.Key
		case "replace":
			if item.Key.Value == "" || item.Key.Confirm {
				return invalidAI("替换密钥须填写新值")
			}
			p.Key = chat.ModelKey(item.Key.Value)
		case "clear":
			if !item.Key.Confirm || item.Key.Value != "" {
				return invalidAI("请确认清除密钥")
			}
		default:
			return invalidAI("密钥操作无效")
		}
		if !exists && p.Type != 1 && p.Key == "" {
			return invalidAI("新建非 OLLaMA 配置需要密钥")
		}
		profiles = append(profiles, p)
	}
	_, err = s.repository.UpdateModels(update.Revision, profiles, update.Routes)
	if errors.Is(err, chat.ErrInvalidModelConfig) {
		return invalidAI(err.Error())
	}
	return err
}

func (s *Server) aiModels(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AI == nil || s.cfg.AI.repository == nil {
		apiError(w, 409, "当前程序未接入 AI 设置")
		return
	}
	if r.Method == http.MethodPost {
		var update modelsUpdate
		if !readJSONLimit(w, r, &update, 131072) {
			return
		}
		if s.cfg.Logs != nil {
			for _, p := range update.Profiles {
				s.cfg.Logs.AddSecrets(p.Key.Value)
			}
		}
		if err := s.cfg.AI.SaveModels(update); err != nil {
			s.aiSaveError(w, err)
			return
		}
	}
	c, revision, err := s.cfg.AI.repository.Read()
	if err != nil {
		apiError(w, 500, "模型配置读取失败")
		return
	}
	profiles, routes := c.ModelCatalog()
	views := make([]modelProfileView, 0, len(profiles))
	for _, p := range profiles {
		api, redacted := chat.DisplayAPI(p.API)
		views = append(views, modelProfileView{ID: p.ID, Name: c.Redact(p.Name), Type: p.Type, Model: c.Redact(p.Model), API: c.Redact(api), KeyConfigured: p.Key != "", AddressRedacted: redacted, Parameters: p.Parameters, EffectiveParameters: p.Parameters.Resolve(c.GlobalParameters())})
	}
	value := map[string]any{"named": c.ModelRoutes != nil, "revision": revision, "profiles": views, "routes": routes, "parameterDefaults": c.GlobalParameters()}
	value["entries"] = modelRouteViews(profiles, routes, c.GlobalParameters())
	if view, err := s.cfg.AI.Get(0); err == nil {
		value["global"] = view.Global
	}
	jsonReply(w, 200, value)
}
