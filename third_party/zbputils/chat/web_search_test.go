package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

type searchTransport func(*http.Request) (*http.Response, error)

func (f searchTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func searchConfig() AgentConfig {
	return AgentConfig{WebSearch: &WebSearchConfig{Enabled: true, Key: "synthetic-search-key", Scopes: map[string]WebSearchPolicy{"456:10": {Enabled: true, Roles: []string{"owner"}}}}}
}
func mockSearch(s *WebSearch, handler http.HandlerFunc) func() {
	server := httptest.NewServer(handler)
	s.client.Transport = searchTransport(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		copyURL := *r.URL
		clone.URL = &copyURL
		clone.URL.Scheme = "http"
		clone.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return server.Client().Transport.RoundTrip(clone)
	})
	return server.Close
}
func fiveSearchResults() string {
	items := []any{}
	for i := 1; i <= 6; i++ {
		items = append(items, map[string]string{"title": fmt.Sprint("资料", i), "url": fmt.Sprintf("https://example.com/source/%d", i), "snippet": "公开资料"})
	}
	b, _ := json.Marshal(map[string]any{"results": items})
	return string(b)
}

func TestWebSearchPolicyAndValidation(t *testing.T) {
	c := searchConfig()
	for _, tc := range []struct {
		bot, conv int64
		role      goba.PermRole
		want      bool
	}{{456, 10, goba.PermRoleOwner, true}, {456, -10, goba.PermRoleOwner, false}, {457, 10, goba.PermRoleOwner, false}, {456, 11, goba.PermRoleOwner, false}, {456, 10, goba.PermRoleAdmin, false}, {456, 10, goba.PermRoleUser, false}} {
		if got := c.SearchAllowed(tc.bot, tc.conv, tc.role); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	c.WebSearch.Scopes["456:-10"] = WebSearchPolicy{Enabled: true, Roles: []string{"user"}}
	if !c.SearchAllowed(456, -10, goba.PermRoleUser) || c.SearchAllowed(456, 10, goba.PermRoleUser) {
		t.Fatal("private policy leaked")
	}
	if (AgentConfig{}).SearchAllowed(456, 10, goba.PermRoleOwner) {
		t.Fatal("old config enabled search")
	}
	if p := (AgentConfig{}).SearchPolicy(456, 10); p.Enabled || len(p.Roles) != 1 || p.Roles[0] != "owner" {
		t.Fatal(p)
	}
	for _, raw := range []string{`null`, `{"enabled":true}`, `{"key":"a\nb"}`, `{"scopes":{"456:10":{"enabled":true,"roles":[]}}}`, `{"scopes":{"0456:10":{"roles":["owner"]}}}`, `{"scopes":{"456:10":{"roles":["user","user"]}}}`, `{"scopes":{"456:10":{"roles":["system"]}}}`} {
		if err := validateField("WebSearch", json.RawMessage(raw)); err == nil {
			t.Fatal("invalid search accepted", raw)
		}
	}
	c.WebSearch.Enabled = false
	if c.SearchAllowed(456, 10, goba.PermRoleOwner) {
		t.Fatal("global off ignored")
	}
	if got := c.Redact("synthetic-search-key"); strings.Contains(got, "synthetic-search-key") {
		t.Fatal("key not redacted")
	}
}

func TestWebSearchRequestResultsAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
		count            int
	}{
		{"five", fiveSearchResults(), "ok", 200, 5}, {"empty", `{"results":[]}`, "empty", 200, 0},
		{"fewer", `{"results":[{"title":"short","url":"https://example.com/one","summary":"summary"}]}`, "ok", 200, 1},
		{"invalid-links", `{"results":[{"url":"file:///private"},{"url":"javascript:alert(1)"},{"url":"https://user:pass@example.com"}]}`, "error", 200, 0},
		{"malformed", `{"results":`, "error", 200, 0}, {"missing", `{}`, "error", 200, 0}, {"null", `{"results":null}`, "error", 200, 0},
		{"oversize", strings.Repeat("x", 256*1024+1), "error", 200, 0}, {"auth", `synthetic-search-key private provider body`, "error", 401, 0}, {"balance", `private provider body`, "error", 402, 0}, {"rate", `private provider body`, "error", 429, 0}, {"server", `private provider body`, "error", 500, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewTaskWebSearch(searchConfig(), 456, 10, goba.PermRoleOwner, nil)
			calls := 0
			defer mockSearch(s, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != "/v1/tools/search" || r.Header.Get("Authorization") != "Bearer synthetic-search-key" {
					t.Error("request contract changed")
				}
				var b map[string]any
				json.NewDecoder(r.Body).Decode(&b)
				if len(b) != 3 || b["query"] != "中文搜索 site:example.com" || b["count"] != float64(5) || b["need_content"] != false {
					t.Error("unexpected payload", b)
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})()
			page := s.search("中文搜索 site:example.com")
			if page.Status != tc.want || len(page.Results) != tc.count || calls != 1 {
				t.Fatal(page, calls)
			}
			encoded, _ := json.Marshal(page)
			if strings.Contains(string(encoded), "synthetic-search-key") || strings.Contains(string(encoded), "private provider body") {
				t.Fatal("provider error exposed")
			}
		})
	}
	s := NewTaskWebSearch(searchConfig(), 456, 10, goba.PermRoleOwner, nil)
	defer mockSearch(s, func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]string{"title": strings.Repeat("中", 200), "url": "https://example.com", "snippet": strings.Repeat("文", 1000)}}})
	})()
	p := s.search("界限")
	if p.Status != "ok" || len(p.Results[0].Title) > 256 || len(p.Results[0].Snippet) > 1600 || !utf8.ValidString(p.Results[0].Snippet) {
		t.Fatal(p)
	}
	s.client.Timeout = time.Millisecond
	s.client.Transport = searchTransport(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	if p := s.search("timeout"); p.Status != "error" {
		t.Fatal(p)
	}
}

func TestWebSearchTaskQuotaPersistenceAndSourceMessages(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	run, err := a.history.tasks.Begin(10)
	if err != nil {
		t.Fatal(err)
	}
	s := NewTaskWebSearch(searchConfig(), 456, 10, goba.PermRoleOwner, run)
	calls := 0
	defer mockSearch(s, func(w http.ResponseWriter, _ *http.Request) { calls++; io.WriteString(w, fiveSearchResults()) })()
	execute := func(params zero.Params) {
		t.Helper()
		if e := run.Action(goba.SearchWeb); e != nil {
			t.Fatal(e)
		}
		if e := s.Execute(&ag, zero.APIRequest{Action: goba.SearchWeb, Params: params}, run.Check); e != nil {
			t.Fatal(e)
		}
		if e := run.Result(s.Outcome()); e != nil {
			t.Fatal(e)
		}
	}
	execute(zero.Params{"query": "公共资料", "group_id": 99})
	if calls != 0 || s.usage.Used != 0 {
		t.Fatal("extra args sent")
	}
	execute(zero.Params{"query": strings.Repeat("中", 128)})
	execute(zero.Params{"query": "second"})
	execute(zero.Params{"query": "third"})
	if calls != 2 || s.usage.Used != 2 || s.usage.Denied != 2 || s.usage.Successful != 2 || run.info.Activity.PlatformActions != 0 || run.info.Queries.Used != 0 {
		t.Fatal(s.usage, run.info)
	}
	if len(s.sources) != 5 {
		t.Fatal("duplicate sources", len(s.sources))
	}
	if reply := s.ReplyIfUnsent(); !strings.Contains(reply, "https://example.com/source/5") || !strings.Contains(reply, "公开资料") {
		t.Fatal("unsent results missing")
	}
	history, err := NewTaskHistorySearch(456, 10, run)
	if err != nil {
		t.Fatal(err)
	}
	history.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) { return ArchivePage{}, nil }
	for i := 0; i < 3; i++ {
		if err = run.Action(goba.SearchHistory); err != nil {
			t.Fatal(err)
		}
		if err = history.Execute(&ag, zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"keyword": fmt.Sprint("本地", i)}}); err != nil {
			t.Fatal(err)
		}
		if err = run.Result(history.Outcome()); err != nil {
			t.Fatal(err)
		}
	}
	if run.info.Queries.Used != 3 || run.info.WebSearch.Used != 2 {
		t.Fatal("local and web quota shared")
	}
	req := s.WithSources(zero.APIRequest{Action: "send_group_msg", Params: zero.Params{"group_id": int64(10), "message": message.Message{message.At(123), message.Text("搜索结论 https://example.com/source/1")}}})
	raw, _ := json.Marshal(req.Params["message"])
	segments := message.ParseMessage(raw)
	text := segments.ExtractPlainText()
	if req.Params["group_id"] != int64(10) || segments[0].Type != "at" || strings.Count(text, "https://example.com/source/1") != 1 || !strings.Contains(text, "https://example.com/source/5") || strings.Contains(text, "/source/6") {
		t.Fatal(text)
	}
	run.Finish(nil)
	page, err := a.history.tasks.PageFiltered(context.Background(), 10, 0, "all")
	if err != nil || page.Items[0].WebSearch.Used != 2 || page.Items[0].WebSearch.Denied != 2 {
		t.Fatal(page, err)
	}
	next, _ := a.history.tasks.Begin(10)
	if next.info.WebSearch.Available || NewTaskWebSearch(searchConfig(), 456, 10, goba.PermRoleOwner, next).usage.Used != 0 {
		t.Fatal("new task quota shared")
	}
	next.Finish(nil)
	denied := NewTaskWebSearch(searchConfig(), 456, -10, goba.PermRoleOwner, nil)
	if !errors.Is(denied.Execute(&ag, zero.APIRequest{Action: goba.SearchWeb}, func() error { return nil }), goba.ErrPermissionDenied) {
		t.Fatal("disabled scope allowed")
	}
}

func TestWebSearchFailureAndEmptyStillConsumeQuota(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	s := NewTaskWebSearch(searchConfig(), 456, 10, goba.PermRoleOwner, nil)
	calls := 0
	defer mockSearch(s, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(500)
			io.WriteString(w, "private error")
			return
		}
		io.WriteString(w, `{"results":[]}`)
	})()
	for i := 0; i < 3; i++ {
		if err := s.Execute(&ag, zero.APIRequest{Action: goba.SearchWeb, Params: zero.Params{"query": fmt.Sprint("public", i)}}, func() error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 || s.usage.Used != 2 || s.usage.Errors != 1 || s.usage.Successful != 1 || s.usage.Empty != 1 || s.usage.Denied != 1 || len(s.sources) != 0 {
		t.Fatal(s.usage, calls)
	}
	if !strings.Contains(s.ReplyIfUnsent(), "本次未取得搜索资料") {
		t.Fatal("unsent failure hidden")
	}
}

func TestWebSearchStopDiscardsResults(t *testing.T) {
	for _, stop := range []string{"cancel", "reload"} {
		t.Run(stop, func(t *testing.T) {
			a, _ := memoryFixture(t)
			ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
			ag.SetHistoryStore(a.history)
			run, _ := a.history.tasks.Begin(10)
			s := NewTaskWebSearch(searchConfig(), 456, 10, goba.PermRoleOwner, run)
			stopped := false
			defer mockSearch(s, func(w http.ResponseWriter, _ *http.Request) { stopped = true; io.WriteString(w, fiveSearchResults()) })()
			run.Action(goba.SearchWeb)
			err := s.Execute(&ag, zero.APIRequest{Action: goba.SearchWeb, Params: zero.Params{"query": "public"}}, func() error {
				if stopped {
					if stop == "reload" {
						return ErrAgentReloaded
					}
					return ErrAgentCancelled
				}
				return run.Check()
			})
			if err == nil || len(s.sources) != 0 || run.info.WebSearch.Used != 1 || run.info.WebSearch.Successful != 1 {
				t.Fatal(err, s.usage)
			}
			rows, _, _ := a.history.recent(10)
			for _, r := range rows {
				if strings.Contains(r.payload, "example.com/source") {
					t.Fatal("cancelled content recorded")
				}
			}
			run.Result(s.Outcome())
			run.Finish(err)
		})
	}
}

func TestWebSearchAgentRoundTripAndSources(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	s := NewTaskWebSearch(searchConfig(), 456, 10, goba.PermRoleOwner, nil)
	defer mockSearch(s, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, fiveSearchResults()) })()
	rounds := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		body, _ := io.ReadAll(r.Body)
		reply := `{"action":"search_web","params":{"query":"公开资料"}}`
		if !strings.Contains(string(body), "search_web") {
			t.Error("missing tool")
		}
		if rounds == 2 {
			if !strings.Contains(string(body), "https://example.com/source/5") || strings.Contains(string(body), "https://example.com/source/6") {
				t.Error("results not carried into next model round")
			}
			reply = `{"action":"send_group_msg","params":{"group_id":10,"message":"找到公开资料"}}` + `{"action":"end_action","params":{}}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": reply}}}})
	}))
	defer server.Close()
	api := deepinfra.NewAPI(server.URL, "fixture")
	api.SetHTTPClient(server.Client())
	sends := 0
	err := RunAgent(func(iter int) ([]zero.APIRequest, bool, error) {
		return NextAgentActionsChecked(&ag, true, iter, api, model.NewOpenAI("fixture", "", .7, 1, 1024, ""), 10, goba.PermRoleOwner, func() error { return nil }, goba.Config{WebSearch: true})
	}, func(req zero.APIRequest) error {
		if req.Action == goba.SearchWeb {
			return s.Execute(&ag, req, func() error { return nil })
		}
		req = s.WithSources(req)
		sends++
		b, _ := json.Marshal(req.Params["message"])
		if !strings.Contains(string(b), "https://example.com/source/5") {
			t.Fatal("source missing from outgoing message")
		}
		return nil
	}, func() bool { return true })
	if err != nil || rounds != 2 || sends != 1 {
		t.Fatal(err, rounds, sends)
	}
}
