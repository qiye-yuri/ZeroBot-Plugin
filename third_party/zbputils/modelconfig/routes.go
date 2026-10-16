// Package modelconfig resolves saved model references without bot initialization.
package modelconfig

import "errors"

type Profile struct {
	ID         string      `json:"id"`
	Type       int         `json:"type"`
	Model      string      `json:"model"`
	API        string      `json:"api"`
	Key        string      `json:"key"`
	Parameters *Parameters `json:"parameters,omitempty"`
}

type Routes struct {
	Chat    string `json:"chat"`
	GPT     string `json:"gpt,omitempty"`
	Summary string `json:"summary,omitempty"`
	Agent   string `json:"agent"`
	Image   string `json:"image"`
}

// Select returns zero profiles for disabled optional entrances. Explicit invalid
// references are errors and can never fall back to a legacy configuration.
func Select(profiles []Profile, routes Routes) (map[string]Profile, error) {
	byID := map[string]Profile{}
	for _, p := range profiles {
		if err := p.Parameters.Validate(); err != nil {
			return nil, err
		}
		if _, ok := byID[p.ID]; ok {
			return nil, errors.New("模型配置编号重复")
		}
		byID[p.ID] = p
	}
	if routes.Chat == "" {
		return nil, errors.New("聊天入口必须选择模型配置")
	}
	result := map[string]Profile{}
	for role, id := range map[string]string{"": routes.Chat, "GPT": routes.GPT, "Summary": routes.Summary, "Agent": routes.Agent, "Image": routes.Image} {
		if id == "" && (role == "GPT" || role == "Summary") {
			id = routes.Chat
		}
		if id == "" {
			result[role] = Profile{}
			continue
		}
		p, ok := byID[id]
		if !ok {
			return nil, errors.New("入口引用的命名模型不存在，请重新选择")
		}
		if p.API == "" || p.Model == "" || (p.Type != 1 && p.Key == "") {
			return nil, errors.New("入口引用的命名模型配置不完整，请核对地址、模型与密钥")
		}
		result[role] = p
	}
	return result, nil
}
