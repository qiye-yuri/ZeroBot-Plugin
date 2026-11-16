package aichat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FloatTech/ZeroBot-Plugin/internal/modelstats"
	"github.com/FloatTech/zbputils/chat"
)

const deepSeekToolEndpoint = "https://api.deepseek.com/chat/completions"
const deepSeekToolResponseLimit = 2 << 20

var errDeepSeekToolTransport = errors.New("DeepSeek tool request failed")

// Only the selected chat entrance supplies credentials; search permissions are
// checked by the caller and never supply a fallback model or key.
func selectDeepSeekToolModel(cfg chat.AgentConfig, agent bool) (chat.AgentConfig, modelstats.Kind, bool) {
	route, kind := "", modelstats.Chat
	if agent {
		route, kind = "Agent", modelstats.Agent
	}
	selected := cfg.ForModel(route)
	u, err := url.Parse(selected.API)
	valid := err == nil && u.Scheme == "https" && strings.EqualFold(u.Host, "api.deepseek.com") && u.User == nil && u.RawQuery == "" && u.Fragment == "" &&
		(u.Path == "" || u.Path == "/" || u.Path == "/v1" || u.Path == "/v1/" || u.Path == "/chat/completions" || u.Path == "/v1/chat/completions") &&
		selected.Type == 0 && selected.ModelName == "deepseek-flash" && selected.Key != ""
	return selected, kind, valid
}

// requestDeepSeekTool performs exactly one bounded HTTP request. Tool payloads,
// admission, user-facing errors and result validation belong to each feature.
// Non-200 responses return their status without reading the provider error body.
func requestDeepSeekTool(ctx context.Context, client *http.Client, selected chat.AgentConfig, kind modelstats.Kind, bot, conversation int64, body []byte, timeout time.Duration) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deepSeekToolEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, errDeepSeekToolTransport
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+string(selected.Key))
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	bounded := *client
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	tracked := modelstats.Default.Client(&bounded, kind, modelstats.Labels{Model: selected.ModelName, Session: fmt.Sprint(bot, ":", conversation)})
	resp, err := tracked.Do(req)
	if err != nil {
		return nil, 0, errDeepSeekToolTransport
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, deepSeekToolResponseLimit+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(raw) > deepSeekToolResponseLimit {
		return nil, resp.StatusCode, errors.New("DeepSeek tool response too large")
	}
	return raw, resp.StatusCode, nil
}
