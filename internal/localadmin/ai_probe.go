package localadmin

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/FloatTech/ZeroBot-Plugin/internal/modelrequest"
	"github.com/FloatTech/zbputils/chat"
	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
)

const modelProbePrompt = "这是后台模型连通性测试。请只回复 OK。"

type modelProbeRequest struct {
	Entry    string `json:"entry"`
	Revision string `json:"revision"`
}

type modelProbeResult struct {
	Entry        string `json:"entry"`
	Model        string `json:"model"`
	Revision     string `json:"revision"`
	CheckedAt    string `json:"checkedAt"`
	Milliseconds int64  `json:"milliseconds"`
	Success      bool   `json:"success"`
	Category     string `json:"category"`
	Message      string `json:"message"`
	HTTPStatus   int    `json:"httpStatus,omitempty"`
}

// Only a single owner-triggered diagnostic is admitted per server. This is
// separate from the archive read gate and never queues or retries requests.
func (s *Server) testAIModel(w http.ResponseWriter, r *http.Request) {
	var req modelProbeRequest
	if !readJSONLimit(w, r, &req, 2048) {
		return
	}
	if (req.Entry != "chat" && req.Entry != "agent") || !validRevision(req.Revision) {
		apiError(w, 400, "请选择聊天或 Agent 入口，并重新读取有效配置版本")
		return
	}
	if s.cfg.AI == nil || s.cfg.AI.repository == nil {
		apiError(w, 409, "当前程序未接入 AI 设置")
		return
	}
	cfg, revision, err := s.cfg.AI.repository.Read()
	if err != nil {
		apiError(w, 500, "模型配置读取失败")
		return
	}
	if revision != req.Revision {
		apiError(w, 409, "模型配置已变化，请重新读取后测试")
		return
	}
	cfg, err = cfg.ResolveModels()
	if err != nil {
		apiError(w, 400, "命名模型引用无效，请核对已保存配置")
		return
	}
	kind, name, address, key := cfg.Type, cfg.ModelName, cfg.API, cfg.Key
	if req.Entry == "agent" {
		kind, name, address, key = cfg.AgentType, cfg.AgentModelName, cfg.AgentAPI, cfg.AgentKey
	}
	if !checkAIConfig(kind, name, address, string(key), "API", true).Complete {
		apiError(w, 400, "该入口未启用或配置不完整，请核对已保存的类型、模型、地址及密钥")
		return
	}
	s.mu.Lock()
	if s.probeActive {
		s.mu.Unlock()
		apiError(w, 429, "已有模型测试进行中，请等待结束后重试")
		return
	}
	s.probeActive = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.probeActive = false; s.mu.Unlock() }()
	// Other APIs keep the server's 10s write deadline. A diagnostic may wait 30s.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(35 * time.Second))
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 30 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	result := runModelProbe(cfg, req.Entry, revision, kind, name, address, key, client)
	jsonReply(w, 200, result)
}

// The result carries no provider response or request content. Both callers
// (HTTP handler and transport tests) use the same protocol/size/error path.
func runModelProbe(cfg chat.AgentConfig, entry, revision string, kind chat.ModelType, name, address string, key chat.ModelKey, client *http.Client) modelProbeResult {
	start := time.Now()
	result := modelProbeResult{Entry: entry, Model: cfg.Redact(name), Revision: revision, CheckedAt: start.UTC().Format(time.RFC3339Nano), Category: "invalid_response", Message: "模型响应无法解析或没有有效文字"}
	role := ""
	if entry == "agent" {
		role = "Agent"
	}
	params := cfg.RequestParams(role)
	protocol, err := kind.ProtocolForAPI(address, name, .2, params.TopP, 128, params.ReasoningEffort, "")
	if err != nil {
		result.Category = "configuration_error"
		result.Message = "模型协议配置不支持本次测试"
		return result
	}
	bounded := &probeTransport{base: client.Transport}
	copyClient := *client
	copyClient.Transport = bounded
	api := deepinfra.NewAPI(address, string(key))
	api.SetHTTPClient(&copyClient)
	text, err := modelrequest.Request(api, protocol.User(model.NewContentText(modelProbePrompt)))
	result.Milliseconds = time.Since(start).Milliseconds()
	result.HTTPStatus = bounded.status
	if bounded.category != "" {
		result.Category = bounded.category
		result.Message = map[string]string{"network_error": "模型网络连接、读取失败或超时，请检查网络及地址", "http_error": "提供商拒绝请求，请检查密钥、模型权限或额度", "response_limit": "模型响应超过256 KiB，本次基础测试失败"}[bounded.category]
	} else if err == nil && strings.TrimSpace(text) != "" {
		result.Success = true
		result.Category = "ok"
		result.Message = "收到有效非空文字，本次基础调用通过"
	}
	return result
}

type probeTransport struct {
	base     http.RoundTripper
	status   int
	category string
}

func (t *probeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(r)
	if err != nil {
		t.category = "network_error"
		return nil, err
	}
	t.status = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		t.category = "http_error"
		return nil, errors.New("model diagnostic HTTP failure")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (256<<10)+1))
	closeErr := resp.Body.Close()
	if err != nil || closeErr != nil {
		t.category = "network_error"
		return nil, errors.New("model diagnostic read failure")
	}
	if len(raw) > 256<<10 {
		t.category = "response_limit"
		return nil, errors.New("model diagnostic response limit")
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	resp.ContentLength = int64(len(raw))
	return resp, nil
}
