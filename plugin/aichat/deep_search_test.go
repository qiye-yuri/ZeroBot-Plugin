package aichat

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	goba "github.com/fumiama/go-onebot-agent"
)

func deepFixture() []byte {
	return []byte(`{"choices":[{"finish_reason":"stop","message":{"content":"根据两处资料得到的答案。","reasoning_content":"不应展示的内部思考","server_tool_calls":[{"calls":[{"type":"deepseek_web_search","kind":"deep_search","status":"completed","action":{"type":"search"},"sources":[{"url":"https://example.com/a","title":"官方说明"}]}]},{"calls":[{"type":"deepseek_web_search","kind":"deep_search","status":"completed","action":{"type":"open_page"},"page":{"url":"https://example.com/b","title":"补充说明"}},{"type":"deepseek_web_search","kind":"deep_search","status":"completed","action":{"type":"find_in_page"}}]}],"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com/b","title":"不可替代真实来源标题"}},{"type":"url_citation","url_citation":{"url":"javascript:bad","title":"假来源"}}]}}],"usage":{"prompt_tokens":100,"completion_tokens":40,"server_tool_use":{"tool_search":2,"tool_fetch":1,"tool_image_search":0}}}`)
}
func TestDeepSearchRequestAndReferences(t *testing.T) {
	calls := 0
	c := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != deepSeekToolEndpoint || r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Fatal("endpoint or key")
		}
		b, _ := io.ReadAll(r.Body)
		var p map[string]any
		if e := json.Unmarshal(b, &p); e != nil {
			t.Fatal(e)
		}
		if p["thinking"].(map[string]any)["type"] != "enabled" || p["reasoning_effort"] != "high" || len(p["tools"].([]any)) != 1 || p["tools"].([]any)[0].(map[string]any)["type"] != "deepseek_web_search" {
			t.Fatal(p)
		}
		if len(p["messages"].([]any)) != 2 || strings.Contains(string(b), "456") || strings.Contains(string(b), "synthetic-search") {
			t.Fatal("unneeded private context")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(deepFixture()))}, nil
	})}
	out, e := runDeepSearch(context.Background(), linkFixture(), deepSeekToolEvent(), goba.PermRoleOwner, false, "比较公开资料", c)
	if e != nil || calls != 1 || !strings.Contains(out, "https://example.com/b") || !strings.Contains(out, "搜索2次") || strings.Contains(out, "内部思考") || strings.Contains(out, "javascript:") || strings.Contains(out, "假来源") {
		t.Fatal(out, e, calls)
	}
}
func TestDeepSearchFailuresAndPartial(t *testing.T) {
	tests := []struct {
		from, to  string
		wantError bool
		hint      string
	}{
		{`"finish_reason":"stop"`, `"finish_reason":"content_filter"`, true, ""},
		{`"status":"completed"`, `"status":"failed"`, true, ""},
		{`"kind":"deep_search"`, `"kind":"search"`, true, ""},
		{`"finish_reason":"stop"`, `"finish_reason":"length"`, false, "不完整"},
		{`"annotations"`, `"ignored_annotations"`, false, "模型未提供"},
		{`"usage"`, `"ignored_usage"`, false, "未返回"},
	}
	for _, tc := range tests {
		out, e := parseDeepSearch(bytes.ReplaceAll(deepFixture(), []byte(tc.from), []byte(tc.to)))
		if (e != nil) != tc.wantError || (!tc.wantError && !strings.Contains(out, tc.hint)) {
			t.Fatal(out, e, tc)
		}
	}
	partial := bytes.Replace(deepFixture(), []byte(`"type":"find_in_page"`), []byte(`"type":"find_in_page"`), 1)
	partial = bytes.Replace(partial, []byte(`"status":"completed","action":{"type":"find_in_page"}`), []byte(`"status":"failed","action":{"type":"find_in_page"}`), 1)
	if out, e := parseDeepSearch(partial); e != nil || !strings.Contains(out, "部分检索") {
		t.Fatal(out, e)
	}
	for _, raw := range []string{`{}`, `{"error":{"message":"internal"}}`, `{"choices":[]}`, `{"choices":[{"message":{"content":"没有联网的猜测"},"finish_reason":"stop"}]}`} {
		if _, e := parseDeepSearch([]byte(raw)); e == nil {
			t.Fatal(raw)
		}
	}
}
func TestDeepSearchAdmissionAndOldCommands(t *testing.T) {
	for _, s := range []string{"搜索 测试", "联网搜索 测试", "读链接 https://example.com", "深度搜索设置"} {
		if _, ok := deepSearchCommand(s); ok {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"深度搜索 问题", "深度思考搜索　问题"} {
		if q, ok := deepSearchCommand(s); !ok || q != "问题" {
			t.Fatal(q, ok)
		}
	}
	c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("network forbidden")
		return nil, context.Canceled
	})}
	if _, e := runDeepSearch(context.Background(), linkFixture(), deepSeekToolEvent(), goba.PermRoleUser, false, "问题", c); e == nil {
		t.Fatal("role")
	}
	if _, e := runDeepSearch(context.Background(), linkFixture(), deepSeekToolEvent(), goba.PermRoleOwner, false, "", c); e == nil {
		t.Fatal("empty")
	}
	cfg := linkFixture()
	cfg.API = "https://other.example"
	if _, e := runDeepSearch(context.Background(), cfg, deepSeekToolEvent(), goba.PermRoleOwner, false, "问题", c); e == nil {
		t.Fatal("provider")
	}
}
func TestDeepSearchTransportNoRetry(t *testing.T) {
	for _, status := range []int{429, 302, 0} {
		calls := 0
		c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) {
			calls++
			if status == 0 {
				return nil, context.DeadlineExceeded
			}
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://other.example"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		})}
		if _, e := runDeepSearch(context.Background(), linkFixture(), deepSeekToolEvent(), goba.PermRoleOwner, false, "问题", c); e == nil || calls != 1 {
			t.Fatal(e, calls)
		}
	}
}
func TestDeepSearchUnmatchedCitationsMarked(t *testing.T) {
	out, e := parseDeepSearch(deepFixture())
	if e != nil || !strings.Contains(out, "部分引用未能") || strings.Contains(out, "javascript:") {
		t.Fatal(out, e)
	}
}
func TestDeepSearchSourceAndUsageBoundaries(t *testing.T) {
	for _, u := range []string{"javascript:alert(1)", "https://user:secret@example.com", "http://127.0.0.1/x", "http://[::1]/x", "https://localhost./x", "https://host.local/x"} {
		if validDeepSearchURL(u) {
			t.Fatal(u)
		}
	}
	zero := int64(0)
	negative := int64(-1)
	if deepUsageValue(nil) != "未返回" || deepUsageValue(&negative) != "未返回" || deepUsageValue(&zero) != "0" {
		t.Fatal("unknown/zero conflated")
	}
	noSearch := bytes.ReplaceAll(deepFixture(), []byte("https://example.com/a"), []byte("http://127.0.0.1/a"))
	if _, e := parseDeepSearch(noSearch); e == nil {
		t.Fatal("invalid only search source accepted")
	}
}
func TestDeepSearchAgentRoute(t *testing.T) {
	cfg := linkFixture()
	cfg.API = "https://unused.example"
	cfg.AgentAPI = "https://api.deepseek.com/v1/chat/completions"
	cfg.AgentModelName = "deepseek-flash"
	cfg.AgentKey = "synthetic-agent"
	c := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer synthetic-agent" {
			t.Fatal("wrong route")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(deepFixture()))}, nil
	})}
	if _, e := runDeepSearch(context.Background(), cfg, deepSeekToolEvent(), goba.PermRoleOwner, true, "问题", c); e != nil {
		t.Fatal(e)
	}
}
