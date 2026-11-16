package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/FloatTech/zbputils/chat"
	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
	"io"
	"net/http"
	"strings"
	"testing"
)

type linkTransport func(*http.Request) (*http.Response, error)

func (f linkTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func linkFixture() chat.AgentConfig {
	return chat.AgentConfig{API: "https://api.deepseek.com", ModelName: "deepseek-flash", Key: "synthetic-key", WebSearch: &chat.WebSearchConfig{Enabled: true, Key: "synthetic-search", Scopes: map[string]chat.WebSearchPolicy{"456:10": {Enabled: true, Roles: []string{"owner"}}}}}
}
func linkResponse(status, finish, content, code string) string {
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": finish, "message": map[string]any{"content": content, "server_tool_calls": []any{map[string]any{"calls": []any{map[string]any{"type": "deepseek_read_url", "kind": "read_link", "action": map[string]string{"url": "https://example.com/a"}, "status": status, "signature": "synthetic-signature", "error": map[string]string{"code": code}}}}}}}}, "usage": map[string]int{"prompt_tokens": 20, "completion_tokens": 10}})
	return string(b)
}
func TestLinkReadingRequestAndResponse(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != deepSeekToolEndpoint || r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Fatal("unexpected endpoint/auth")
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if e := json.Unmarshal(raw, &body); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(raw), "456") || strings.Contains(string(raw), "synthetic-search") {
			t.Fatal("identity or unrelated key leaked")
		}
		if len(body["tools"].([]any)) != 1 || body["tools"].([]any)[0].(map[string]any)["type"] != "deepseek_read_url" || body["thinking"].(map[string]any)["type"] != "disabled" {
			t.Fatal("incorrect tool/mode")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(linkResponse("completed", "stop", "网页总结", "")))}, nil
	})}
	out, e := readLink(context.Background(), linkFixture(), false, 456, 10, goba.PermRoleOwner, "https://example.com/a 总结", client)
	if e != nil || calls != 1 || !strings.Contains(out, "网页总结") || !strings.Contains(out, "来源：https://example.com/a") || strings.Contains(out, "signature") {
		t.Fatal(out, e, calls)
	}
}
func TestLinkReadingFailuresNeverRetry(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"inaccessible", linkResponse("failed", "stop", "不能展示", "url_not_accessible"), 200, "无法读取"},
		{"blocked", linkResponse("failed", "stop", "不能展示", "url_not_allowed"), 200, "不允许"},
		{"refusal", linkResponse("completed", "content_filter", "不能展示", ""), 200, "审核"},
		{"no tool", `{"choices":[{"finish_reason":"stop","message":{"content":"猜测正文"}}]}`, 200, "未确认"},
		{"server error", `{"error":{"message":"secret internal detail"}}`, 200, "有效结果"},
		{"quota", "", 429, "HTTP 429"},
		{"malformed", "broken", 200, "有效结果"},
		{"empty", linkResponse("completed", "stop", "", ""), 200, "未返回"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			out, e := readLink(context.Background(), linkFixture(), false, 456, 10, goba.PermRoleOwner, "https://example.com/a", c)
			if e == nil || !strings.Contains(e.Error(), tc.want) || out != "" || calls != 1 {
				t.Fatal(out, e, calls)
			}
		})
	}
}
func TestLinkReadingAdmissionNoNetwork(t *testing.T) {
	c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("network must not run")
		return nil, errors.New("blocked")
	})}
	for _, input := range []string{"", "http://127.0.0.1/a", "http://localhost/x", "file:///x", "https://user:pass@example.com", "https://example.com/a https://other.com/b"} {
		if _, e := readLink(context.Background(), linkFixture(), false, 456, 10, goba.PermRoleOwner, input, c); e == nil {
			t.Fatal(input)
		}
	}
	cfg := linkFixture()
	if _, e := readLink(context.Background(), cfg, false, 456, 10, goba.PermRoleUser, "https://example.com/a", c); e == nil {
		t.Fatal("permission")
	}
	cfg.API = "https://other.example"
	if _, e := readLink(context.Background(), cfg, false, 456, 10, goba.PermRoleOwner, "https://example.com/a", c); e == nil {
		t.Fatal("provider")
	}
	cfg = linkFixture()
	cfg.WebSearch.Enabled = false
	if _, e := readLink(context.Background(), cfg, false, 456, 10, goba.PermRoleOwner, "https://example.com/a", c); e == nil {
		t.Fatal("disabled")
	}
}
func TestLinkCommand(t *testing.T) {
	for _, s := range []string{"读链接", "读链接 https://example.com", "读链接\nhttps://example.com"} {
		if _, ok := linkCommand(s); !ok {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"请读链接", "https://example.com", "读链接设置"} {
		if _, ok := linkCommand(s); ok {
			t.Fatal(s)
		}
	}
}
func TestLinkReadingAgentRouteAndTruncation(t *testing.T) {
	cfg := linkFixture()
	cfg.API = "https://unused.example"
	cfg.AgentAPI = "https://api.deepseek.com/v1"
	cfg.AgentModelName = "deepseek-flash"
	cfg.AgentKey = "synthetic-agent"
	c := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer synthetic-agent" {
			t.Fatal("wrong route")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(linkResponse("completed", "length", "部分回答", "")))}, nil
	})}
	out, e := readLink(context.Background(), cfg, true, 456, 10, goba.PermRoleOwner, "https://example.com/a", c)
	if e != nil || !strings.Contains(out, "不完整") {
		t.Fatal(out, e)
	}
}
func TestLinkReadingTransportFailure(t *testing.T) {
	calls := 0
	c := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if _, ok := r.Context().Deadline(); !ok {
			t.Fatal("missing deadline")
		}
		return nil, context.DeadlineExceeded
	})}
	out, e := readLink(context.Background(), linkFixture(), false, 456, 10, goba.PermRoleOwner, "https://example.com/a", c)
	if e == nil || out != "" || calls != 1 {
		t.Fatal(out, e, calls)
	}
}

func TestLinkRequiresActualGroupMention(t *testing.T) {
	for _, tc := range []struct {
		group  int64
		native string
		want   bool
	}{
		{0, "", true},
		{10, "读链接 https://example.com/bot-name", false},
		{10, "[CQ:at,qq=999]读链接 https://example.com", false},
		{10, "[CQ:at,qq=456]读链接 https://example.com", true},
		{10, "[{\"type\":\"at\",\"data\":{\"qq\":\"456\"}}]", true},
	} {
		native := []byte(tc.native)
		if !strings.HasPrefix(tc.native, "[{") {
			native, _ = json.Marshal(tc.native)
		}
		if got := linkAddressed(&zero.Event{GroupID: tc.group, SelfID: 456, NativeMessage: native, IsToMe: true}); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}
func TestLinkWhitespace(t *testing.T) {
	input, ok := linkCommand("读链接　https://example.com/a 问题")
	if !ok {
		t.Fatal("unicode command space")
	}
	u, q, e := parseLinkInput("  " + input + "  ")
	if e != nil || u != "https://example.com/a" || q != "问题" {
		t.Fatal(u, q, e)
	}
}

func TestLinkReadingFullEndpointConfiguration(t *testing.T) {
	for _, path := range []string{"/chat/completions", "/v1/chat/completions"} {
		cfg := linkFixture()
		cfg.API = "https://api.deepseek.com" + path
		calls := 0
		c := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.String() != deepSeekToolEndpoint {
				t.Fatal("wrong target")
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(linkResponse("completed", "stop", "测试回答", "")))}, nil
		})}
		if _, e := readLink(context.Background(), cfg, false, 456, 10, goba.PermRoleOwner, "https://example.com/a", c); e != nil || calls != 1 {
			t.Fatal(path, e, calls)
		}
	}
}
