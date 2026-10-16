package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

type ModelType int

var apilist = [3]string{"OpenAI", "OLLaMA", "GenAI"}

func newModelType(text string) (ModelType, error) {
	for i, name := range apilist {
		if text == name {
			return ModelType(i), nil
		}
	}
	return 0, errors.New("未知接口类型")
}
func (mt ModelType) String() string {
	if mt < 0 || int(mt) >= len(apilist) {
		return "未知类型"
	}
	return apilist[mt]
}
func (mt ModelType) Protocol(name string, temp, topp float32, maxn uint, reasoning string, separators ...string) (model.Protocol, error) {
	separator := ""
	if len(separators) > 0 {
		separator = separators[0]
	}
	switch mt {
	case 0:
		return model.NewOpenAI(name, separator, temp, topp, maxn, reasoning), nil
	case 1:
		return model.NewOLLaMA(name, separator, temp, topp, maxn), nil
	case 2:
		return model.NewGenAI(name, temp, topp, maxn), nil
	default:
		return nil, errors.New("不支持的模型接口类型")
	}
}

type ModelBool bool

func (b ModelBool) String() string {
	if b {
		return "是"
	}
	return "否"
}

type ModelKey string

func (k ModelKey) String() string {
	if k == "" {
		return "未设置"
	}
	return "已配置（隐藏）"
}

// AgentConfig preserves the existing on-disk JSON field names. Callers use
// immutable snapshots from the shared control database instead of a global AC.
type AgentConfig struct {
	ModelName, ImageModelName, AgentModelName string
	Type, ImageType, AgentType                ModelType
	MaxN                                      uint
	TopP                                      float32
	SystemP, AgentChar, AgentSex              string
	API, ImageAPI, AgentAPI                   string
	Key, ImageKey, AgentKey                   ModelKey
	Separator, ReasoningEffort                string
	NoSystemP                                 ModelBool
	Personas                                  map[string]AgentPersona `json:",omitempty"`
	ModelProfiles                             []ModelProfile          `json:",omitempty"`
	ModelRoutes                               *ModelRoutes            `json:",omitempty"`
}

func newconfig() AgentConfig {
	return AgentConfig{ModelName: model.ModelDeepDeek, SystemP: SystemPrompt, API: deepinfra.OpenAIDeepInfra, AgentChar: agentcharcfg.Char, AgentSex: agentcharcfg.Sex}
}
func (c AgentConfig) MParams() (float32, uint) {
	top, max := c.TopP, c.MaxN
	if top == 0 {
		top = 0.9
	}
	if max == 0 {
		max = 4096
	}
	return top, max
}
func (c AgentConfig) Ready() bool {
	return c.Type >= 0 && c.Type <= 2 && c.API != "" && c.ModelName != "" && (c.Key != "" || c.Type == 1)
}

// DisplayAPI hides all URL credentials and query values in legacy settings.
func DisplayAPI(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "地址无效（请替换）", true
	}
	redacted := u.User != nil || u.RawQuery != "" || u.Fragment != ""
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	if redacted {
		return u.String() + "（认证参数已隐藏）", true
	}
	return u.String(), false
}
func (c AgentConfig) Redact(text string) string {
	keys := []ModelKey{c.Key, c.ImageKey, c.AgentKey}
	for _, profile := range c.ModelProfiles {
		keys = append(keys, profile.Key)
	}
	for _, key := range keys {
		if key != "" {
			text = strings.ReplaceAll(text, string(key), "[已隐藏]")
		}
	}
	return text
}
func (c AgentConfig) String() string {
	api, _ := DisplayAPI(c.API)
	imageAPI, _ := DisplayAPI(c.ImageAPI)
	agentAPI, _ := DisplayAPI(c.AgentAPI)
	top, max := c.MParams()
	return c.Redact(fmt.Sprintf("• 模型名：%s\n• 图像模型名：%s\n• Agent模型名：%s\n• 接口类型：%v\n• 图像接口类型：%v\n• Agent接口类型：%v\n• 最大长度：%d\n• TopP：%g\n• 系统提示词：%s\n• Agent性格：%s\n• Agent性别：%s\n• 接口地址：%s\n• 图像接口地址：%s\n• Agent接口地址：%s\n• 密钥：%v\n• 图像密钥：%v\n• Agent密钥：%v\n• 分隔符：%s\n• 推理努力度：%s\n• 支持系统提示词：%v\n", c.ModelName, c.ImageModelName, c.AgentModelName, c.Type, c.ImageType, c.AgentType, max, top, c.SystemP, c.AgentChar, c.AgentSex, api, imageAPI, agentAPI, c.Key, c.ImageKey, c.AgentKey, c.Separator, c.ReasoningEffort, !c.NoSystemP))
}

type ConfigRepository struct{ Control *ctrl.Control[*zero.Ctx] }

func NewConfigRepository(c *ctrl.Control[*zero.Ctx]) *ConfigRepository {
	return &ConfigRepository{Control: c}
}
func decodeAgentConfig(raw json.RawMessage) (AgentConfig, error) {
	c := newconfig()
	if len(raw) > 0 {
		if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
			return AgentConfig{}, errors.New("AI 配置格式无效")
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return AgentConfig{}, errors.New("AI 配置格式无效")
		}
	}
	return c, nil
}
func (r *ConfigRepository) Read() (AgentConfig, string, error) {
	if r == nil || r.Control == nil {
		return AgentConfig{}, "", errors.New("AI 配置插件未编译")
	}
	raw, revision, err := r.Control.ExtraJSON()
	if err != nil {
		return AgentConfig{}, "", err
	}
	c, err := decodeAgentConfig(raw)
	return c, revision, err
}

// validateField validates only changed fields so unrelated legacy values are
// preserved; new URLs must place authentication in the separate key field.
func validateField(name string, value json.RawMessage) error {
	if name == "ModelProfiles" {
		var profiles []ModelProfile
		if json.Unmarshal(value, &profiles) != nil || profiles == nil {
			return errors.New("命名模型列表格式无效")
		}
		for _, p := range profiles {
			if err := p.Validate(); err != nil {
				return err
			}
		}
		return nil
	}
	if name == "ModelRoutes" {
		var routes ModelRoutes
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, &routes) != nil {
			return errors.New("模型入口格式无效")
		}
		return nil
	}
	if name == "Personas" {
		var personas map[string]AgentPersona
		if json.Unmarshal(value, &personas) != nil || personas == nil {
			return errors.New("会话人物设置格式无效")
		}
		for key, persona := range personas {
			if !validPersonaKey(key) {
				return errors.New("会话人物范围无效")
			}
			if err := persona.Validate(); err != nil {
				return err
			}
		}
		return nil
	}
	typ, ok := reflect.TypeFor[AgentConfig]().FieldByName(name)
	if !ok {
		return errors.New("未知 AI 设置")
	}
	ptr := reflect.New(typ.Type)
	if bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, ptr.Interface()) != nil {
		return errors.New("AI 设置值类型无效")
	}
	v := ptr.Elem()
	switch v.Kind() {
	case reflect.String:
		text := v.String()
		max := 256
		switch name {
		case "SystemP", "AgentChar":
			max = 32768
		case "API", "ImageAPI", "AgentAPI":
			max = 2048
		case "Key", "ImageKey", "AgentKey":
			max = 4096
		}
		if len(text) > max || !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') {
			return errors.New("AI 文本过长或包含无效字符")
		}
		if strings.HasSuffix(name, "Key") && strings.ContainsAny(text, "\r\n") {
			return errors.New("密钥不能包含换行")
		}
		if name == "ModelName" && strings.TrimSpace(text) == "" {
			return errors.New("聊天模型名不能为空")
		}
		if name == "API" || name == "ImageAPI" || name == "AgentAPI" {
			if text == "" && name != "API" {
				break
			}
			u, err := url.Parse(text)
			if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
				return errors.New("接口须为完整 HTTP/HTTPS 地址，密钥请填写在独立字段")
			}
			if port := u.Port(); port != "" {
				n, err := strconv.ParseUint(port, 10, 16)
				if err != nil || n == 0 {
					return errors.New("接口端口须为 1～65535")
				}
			}
		}
	case reflect.Int:
		if v.Int() < 0 || v.Int() > 2 {
			return errors.New("接口类型须为 OpenAI、OLLaMA 或 GenAI")
		}
	case reflect.Uint:
		if v.Uint() > 131072 {
			return errors.New("最大长度须为 0～131072，0 使用默认值")
		}
	case reflect.Float32:
		n := v.Float()
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1 {
			return errors.New("TopP 须为 0～1，0 使用默认值")
		}
	}
	return nil
}

func ValidateConfigFields(fields map[string]json.RawMessage) error {
	for name, value := range fields {
		if err := validateField(name, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *ConfigRepository) Update(revision string, fields map[string]json.RawMessage) (string, error) {
	if r == nil || r.Control == nil {
		return "", errors.New("AI 配置插件未编译")
	}
	changed := false
	saved, err := r.Control.UpdateExtraJSON(revision, func(raw json.RawMessage) (json.RawMessage, error) {
		current, err := decodeAgentConfig(raw)
		if err != nil {
			return nil, err
		}
		if current.ModelRoutes != nil {
			for name := range fields {
				if modelField(name) {
					return nil, ErrNamedModelFields
				}
			}
		}
		if len(fields) == 0 {
			return raw, nil
		}
		obj := map[string]json.RawMessage{}
		if len(raw) > 0 {
			if json.Unmarshal(raw, &obj) != nil || obj == nil {
				return nil, errors.New("AI 配置格式无效")
			}
		}
		for name, value := range fields {
			if err := validateField(name, value); err != nil {
				if name == "ModelProfiles" || name == "ModelRoutes" {
					return nil, fmt.Errorf("%w：%s", ErrInvalidModelConfig, err)
				}
				return nil, err
			}
			for oldName := range obj {
				if strings.EqualFold(name, oldName) {
					delete(obj, oldName)
				}
			}
			obj[name] = value
		}
		result, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		candidate, err := decodeAgentConfig(result)
		if err != nil {
			return nil, err
		}
		if err = candidate.validateModelRoutes(); err != nil {
			return nil, fmt.Errorf("%w：%s", ErrInvalidModelConfig, err)
		}
		changed = !bytes.Equal(raw, result)
		return result, err
	})
	_, personaOnly := fields["Personas"]
	if err == nil && changed && !(personaOnly && len(fields) == 1) {
		ReloadAgents()
	}
	return saved, err
}

func globalRepository() *ConfigRepository {
	c, ok := control.Lookup("aichatcfg")
	if !ok {
		c, _ = control.Lookup("aichat")
	}
	return NewConfigRepository(c)
}
func CurrentConfig() (AgentConfig, error) {
	c, _, err := globalRepository().Read()
	if err != nil {
		return c, err
	}
	return c.ResolveModels()
}

const configStateKey = zero.StateKeyPrefixKeep + "chat_config_snapshot__"

func EnsureConfig(ctx *zero.Ctx) bool {
	if _, ok := ctx.State[configStateKey].(AgentConfig); ok {
		return true
	}
	c, err := CurrentConfig()
	if err != nil {
		return false
	}
	ctx.State[configStateKey] = c
	return true
}
func ConfigOf(ctx *zero.Ctx) AgentConfig { c, _ := ctx.State[configStateKey].(AgentConfig); return c }
func saveFields(ctx *zero.Ctx, fields map[string]json.RawMessage) {
	if _, err := globalRepository().Update("", fields); err != nil {
		if errors.Is(err, ErrNamedModelFields) {
			ctx.SendChain(message.Text(ErrNamedModelFields.Error()))
			return
		}
		ctx.SendChain(message.Text("AI 配置保存失败，原设置保留。"))
		return
	}
	ctx.SendChain(message.Text("成功"))
}
func NewExtraSetStr(field string) func(*zero.Ctx) {
	return func(ctx *zero.Ctx) {
		raw, _ := json.Marshal(strings.TrimSpace(ctx.State["args"].(string)))
		saveFields(ctx, map[string]json.RawMessage{field: raw})
	}
}
func NewExtraSetBool(field string) func(*zero.Ctx) {
	return func(ctx *zero.Ctx) {
		raw, _ := json.Marshal(ctx.State["regex_matched"].([]string)[1] == "不")
		saveFields(ctx, map[string]json.RawMessage{field: raw})
	}
}
func NewExtraSetUint(field string) func(*zero.Ctx) {
	return func(ctx *zero.Ctx) {
		n, err := strconv.ParseUint(strings.TrimSpace(ctx.State["args"].(string)), 10, 64)
		if err != nil {
			ctx.SendChain(message.Text("请输入合法的非负整数。"))
			return
		}
		raw, _ := json.Marshal(n)
		saveFields(ctx, map[string]json.RawMessage{field: raw})
	}
}
func NewExtraSetFloat32(field string) func(*zero.Ctx) {
	return func(ctx *zero.Ctx) {
		n, err := strconv.ParseFloat(strings.TrimSpace(ctx.State["args"].(string)), 32)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			ctx.SendChain(message.Text("请输入合法数值。"))
			return
		}
		raw, _ := json.Marshal(n)
		saveFields(ctx, map[string]json.RawMessage{field: raw})
	}
}
func NewExtraSetModelType(field string) func(*zero.Ctx) {
	return func(ctx *zero.Ctx) {
		t, err := newModelType(strings.TrimSpace(ctx.State["args"].(string)))
		if err != nil {
			ctx.SendChain(message.Text("未知接口类型。"))
			return
		}
		raw, _ := json.Marshal(t)
		saveFields(ctx, map[string]json.RawMessage{field: raw})
	}
}
func ResetSystemPrompt(ctx *zero.Ctx) {
	raw, _ := json.Marshal(SystemPrompt)
	saveFields(ctx, map[string]json.RawMessage{"SystemP": raw})
}
func ResetAgentCharacter(ctx *zero.Ctx) {
	c, _ := json.Marshal(agentcharcfg.Char)
	s, _ := json.Marshal(agentcharcfg.Sex)
	saveFields(ctx, map[string]json.RawMessage{"AgentChar": c, "AgentSex": s})
}
