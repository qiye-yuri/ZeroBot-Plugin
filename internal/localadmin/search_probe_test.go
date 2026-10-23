package localadmin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/FloatTech/ZeroBot-Plugin/internal/modelstats"
	"github.com/FloatTech/zbputils/chat"
)

type probeSearchTransport func(*http.Request) (*http.Response, error)

func (f probeSearchTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func useSearchTransport(t *testing.T, f probeSearchTransport) {
	old := http.DefaultTransport
	http.DefaultTransport = f
	t.Cleanup(func() { http.DefaultTransport = old })
}

func TestSearchProbeSavedKeyReadOnlyAndCategories(t *testing.T) {
	for _, tc := range []struct {
		body, category string
		status, count  int
	}{
		{`{"results":[{"url":"https://example.com/private","title":"PRIVATE_TITLE","snippet":"PRIVATE_SNIPPET"}]}`, "success", 200, 1},
		{`{"results":[]}`, "empty", 200, 0}, {`{}`, "invalid_response", 200, 0},
		{`{"results":[{"url":"file:///private"}]}`, "invalid_response", 200, 0},
		{strings.Repeat("x", 256*1024+1), "response_limit", 200, 0},
		{"PRIVATE_PROVIDER_BODY", "http_error", 401, 0}, {"PRIVATE_PROVIDER_BODY", "http_error", 402, 0},
		{"PRIVATE_PROVIDER_BODY", "http_error", 429, 0}, {"PRIVATE_PROVIDER_BODY", "http_error", 503, 0},
	} {
		t.Run(tc.category+http.StatusText(tc.status), func(t *testing.T) {
			s := newTestServer(t)
			store, c, file, backups := aiFixture(t)
			s.cfg.AI = store
			if err := c.SetExtra(chat.AgentConfig{Key: "WRONG_MODEL_KEY", WebSearch: &chat.WebSearchConfig{Key: "SAVED_SEARCH_KEY", Enabled: false}}); err != nil {
				t.Fatal(err)
			}
			_, revision, _ := store.repository.Read()
			before, _ := os.ReadFile(file)
			beforeBackups, _ := os.ReadDir(backups)
			stats := modelstats.Default.Read()
			calls := 0
			useSearchTransport(t, func(r *http.Request) (*http.Response, error) {
				calls++
				var payload map[string]any
				json.NewDecoder(r.Body).Decode(&payload)
				if r.Method != "POST" || r.URL.String() != "https://api.deepseek.com/v1/tools/search" || r.Header.Get("Authorization") != "Bearer SAVED_SEARCH_KEY" || len(payload) != 3 || payload["query"] != searchProbeQuery || payload["count"] != float64(5) || payload["need_content"] != false {
					t.Error("unexpected diagnostic request", r.URL, payload)
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			cookie, csrf := enroll(t, s)
			w := request(s, "POST", "/api/ai-search-test", map[string]any{"revision": revision}, cookie, csrf)
			expectStatus(t, w, 200)
			var result searchProbeResult
			json.Unmarshal(w.Body.Bytes(), &result)
			if result.Category != tc.category || result.ResultCount != tc.count || result.HTTPStatus != tc.status || result.Revision != revision || result.CheckedAt == "" || result.Message == "" {
				t.Fatal(w.Body.String())
			}
			for _, private := range []string{"SAVED_SEARCH_KEY", "WRONG_MODEL_KEY", "PRIVATE_", "example.com"} {
				if strings.Contains(w.Body.String(), private) {
					t.Fatal("private content leaked")
				}
			}
			after, _ := os.ReadFile(file)
			afterBackups, _ := os.ReadDir(backups)
			if calls != 1 || !bytes.Equal(before, after) || len(beforeBackups) != len(afterBackups) || modelstats.Default.Read().Started != stats.Started {
				t.Fatal("probe caused business/configuration changes or retried")
			}
		})
	}
}

func TestSearchProbeAdmissionAndSharedGate(t *testing.T) {
	s := newTestServer(t)
	store, c, _, _ := aiFixture(t)
	s.cfg.AI = store
	cfg := chat.AgentConfig{API: "https://example.invalid", ModelName: "model", Key: "model-key", WebSearch: &chat.WebSearchConfig{Key: "search-key"}}
	if err := c.SetExtra(cfg); err != nil {
		t.Fatal(err)
	}
	_, revision, _ := store.repository.Read()
	body := map[string]any{"revision": revision}
	calls := 0
	entered, release := make(chan struct{}), make(chan struct{})
	useSearchTransport(t, func(r *http.Request) (*http.Response, error) {
		calls++
		close(entered)
		<-release
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"results":[]}`))}, nil
	})
	expectStatus(t, request(s, "POST", "/api/ai-search-test", body, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "POST", "/api/ai-search-test", body, cookie, "wrong"), 403)
	crossOrigin := httptest.NewRequest("POST", "http://"+s.cfg.Addr+"/api/ai-search-test", strings.NewReader(`{"revision":"`+revision+`"}`))
	crossOrigin.RemoteAddr = "127.0.0.1:54321"
	crossOrigin.AddCookie(cookie)
	crossOrigin.Header.Set("Origin", "https://example.invalid")
	crossOrigin.Header.Set("Content-Type", "application/json")
	crossOrigin.Header.Set("X-Local-Request", "1")
	crossOrigin.Header.Set("X-CSRF-Token", csrf)
	denied := httptest.NewRecorder()
	s.Handler().ServeHTTP(denied, crossOrigin)
	expectStatus(t, denied, 403)
	for _, invalid := range []map[string]any{{"revision": "bad"}, {"revision": revision, "key": "draft"}, {"revision": revision, "query": "private"}, {"revision": revision, "url": "http://localhost"}, {"revision": revision, "botId": "12345678"}} {
		expectStatus(t, request(s, "POST", "/api/ai-search-test", invalid, cookie, csrf), 400)
	}
	// A model diagnostic already owns the same gate.
	s.probeActive = true
	expectStatus(t, request(s, "POST", "/api/ai-search-test", body, cookie, csrf), 429)
	s.probeActive = false
	if calls != 0 {
		t.Fatal("rejected request reached provider")
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- request(s, "POST", "/api/ai-search-test", body, cookie, csrf) }()
	<-entered
	expectStatus(t, request(s, "POST", "/api/ai-search-test", body, cookie, csrf), 429)
	expectStatus(t, request(s, "POST", "/api/ai-test", map[string]any{"entry": "chat", "revision": revision}, cookie, csrf), 429)
	cfg.WebSearch.Key = "changed"
	if err := c.SetExtra(cfg); err != nil {
		t.Fatal(err)
	}
	close(release)
	w := <-done
	expectStatus(t, w, 200)
	var result searchProbeResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Revision != revision || calls != 1 || s.probeActive {
		t.Fatal("snapshot/gate failed")
	}
	expectStatus(t, request(s, "POST", "/api/ai-search-test", body, cookie, csrf), 409)
	cfg.WebSearch.Key = ""
	c.SetExtra(cfg)
	_, revision, _ = store.repository.Read()
	expectStatus(t, request(s, "POST", "/api/ai-search-test", map[string]any{"revision": revision}, cookie, csrf), 400)
	if calls != 1 {
		t.Fatal("admission called provider")
	}
}
