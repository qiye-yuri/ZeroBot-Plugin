package localadmin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FloatTech/ZeroBot-Plugin/internal/modelstats"
	"github.com/FloatTech/zbputils/chat"
	"github.com/FloatTech/zbputils/modelconfig"
)

func TestModelProbeSavedProtocolsReadOnlyAndNoBusinessStats(t *testing.T) {
	for _, kind := range []chat.ModelType{0, 1, 2} {
		for _, named := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/named=%t", kind, named), func(t *testing.T) {
				var calls atomic.Int32
				provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					body, _ := io.ReadAll(r.Body)
					var payload map[string]any
					if err := json.Unmarshal(body, &payload); err != nil {
						t.Error(err)
					}
					maxField, topField := "max_tokens", "top_p"
					if kind == 2 {
						payload = payload["generationConfig"].(map[string]any)
						maxField, topField = "maxOutputTokens", "topP"
					}
					wantTop := .9
					if named {
						if calls.Load() == 1 {
							wantTop = .31
						} else {
							wantTop = .62
						}
					}
					if payload[maxField] != float64(128) || payload[topField] != wantTop {
						t.Error("diagnostic parameters", payload)
					}
					if kind == 0 && named {
						wantReasoning := any(nil)
						if calls.Load() == 2 {
							wantReasoning = "low"
						}
						if payload["reasoning_effort"] != wantReasoning {
							t.Error("diagnostic reasoning", payload)
						}
					}
					if !strings.Contains(string(body)+r.URL.String(), "selected-model") || !bytes.Contains(body, []byte(modelProbePrompt)) || bytes.Contains(body, []byte("PRIVATE_SYSTEM")) {
						t.Error("unexpected probe payload")
					}
					responses := []string{`{"choices":[{"message":{"content":"PROVIDER_PRIVATE_TEXT"}}]}`, `{"message":{"content":"PROVIDER_PRIVATE_TEXT"}}`, `{"candidates":[{"content":{"parts":[{"text":"PROVIDER_PRIVATE_TEXT"}]}}]}`}
					io.WriteString(w, responses[int(kind)])
				}))
				defer provider.Close()
				s := newTestServer(t)
				store, c, file, backups := aiFixture(t)
				s.cfg.AI = store
				cfg := chat.AgentConfig{API: provider.URL, ModelName: "selected-model", Key: "SYNTHETIC_SECRET", Type: kind, AgentAPI: provider.URL, AgentModelName: "selected-model", AgentKey: "SYNTHETIC_SECRET", AgentType: kind, SystemP: "PRIVATE_SYSTEM"}
				if named {
					max, chatTop, agentTop, empty, low := uint(900), float32(.31), float32(.62), "", "low"
					cfg.ReasoningEffort = "high"
					cfg.ModelProfiles = []chat.ModelProfile{{ID: "chosen", Model: "selected-model", API: provider.URL, Type: kind, Key: "SYNTHETIC_SECRET", Parameters: &modelconfig.Parameters{MaxN: &max, TopP: &chatTop, ReasoningEffort: &empty}}, {ID: "agent", Model: "selected-model", API: provider.URL, Type: kind, Key: "SYNTHETIC_SECRET", Parameters: &modelconfig.Parameters{MaxN: &max, TopP: &agentTop, ReasoningEffort: &low}}}
					cfg.ModelRoutes = &chat.ModelRoutes{Chat: "chosen", Agent: "agent"}
					cfg.API = "http://127.0.0.1:1"
					cfg.ModelName = "wrong-legacy"
				}
				if err := c.SetExtra(cfg); err != nil {
					t.Fatal(err)
				}
				_, revision, _ := store.repository.Read()
				before, _ := os.ReadFile(file)
				beforeBackups, _ := os.ReadDir(backups)
				stats := modelstats.Default.Read()
				cookie, csrf := enroll(t, s)
				for _, entry := range []string{"chat", "agent"} {
					w := request(s, "POST", "/api/ai-test", map[string]any{"entry": entry, "revision": revision}, cookie, csrf)
					expectStatus(t, w, 200)
					var result modelProbeResult
					if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					if !result.Success || result.Entry != entry || result.Model != "selected-model" || result.Revision != revision || result.CheckedAt == "" {
						t.Fatal(w.Body.String())
					}
					for _, private := range []string{"SYNTHETIC_SECRET", "PROVIDER_PRIVATE_TEXT", "PRIVATE_SYSTEM"} {
						if strings.Contains(w.Body.String(), private) {
							t.Fatal("response disclosed private data")
						}
					}
				}
				after, _ := os.ReadFile(file)
				afterBackups, _ := os.ReadDir(backups)
				if calls.Load() != 2 || !bytes.Equal(before, after) || len(beforeBackups) != len(afterBackups) || modelstats.Default.Read().Started != stats.Started {
					t.Fatal("probe wrote settings, retried or counted as business traffic")
				}
			})
		}
	}
}

func TestModelProbeAdmissionAuthFieldsAndStaleRevision(t *testing.T) {
	s := newTestServer(t)
	store, c, _, _ := aiFixture(t)
	s.cfg.AI = store
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, `{"choices":[{"message":{"content":"OK"}}]}`)
	}))
	defer provider.Close()
	cfg := chat.AgentConfig{API: provider.URL, ModelName: "selected", Key: "synthetic"}
	if err := c.SetExtra(cfg); err != nil {
		t.Fatal(err)
	}
	_, revision, _ := store.repository.Read()
	body := map[string]any{"entry": "chat", "revision": revision}
	expectStatus(t, request(s, "POST", "/api/ai-test", body, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "POST", "/api/ai-test", body, cookie, "bad"), 403)
	for _, invalid := range []map[string]any{{"entry": "image", "revision": revision}, {"entry": "chat", "revision": "bad"}, {"entry": "chat", "revision": revision, "api": provider.URL}, {"entry": "chat", "revision": revision, "prompt": "private"}} {
		expectStatus(t, request(s, "POST", "/api/ai-test", invalid, cookie, csrf), 400)
	}
	expectStatus(t, request(s, "POST", "/api/ai-test", map[string]any{"entry": "agent", "revision": revision}, cookie, csrf), 400)
	if err := c.SetExtra(chat.AgentConfig{API: provider.URL, ModelName: "changed", Key: "synthetic"}); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, request(s, "POST", "/api/ai-test", body, cookie, csrf), 409)
	if err := c.SetExtra(chat.AgentConfig{ModelRoutes: &chat.ModelRoutes{Chat: "missing"}}); err != nil {
		t.Fatal(err)
	}
	_, revision, _ = store.repository.Read()
	body["revision"] = revision
	expectStatus(t, request(s, "POST", "/api/ai-test", body, cookie, csrf), 400)
	if calls.Load() != 0 {
		t.Fatal("invalid admission called provider")
	}
}

func TestModelProbeInFlightGateAndSnapshot(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		close(entered)
		<-release
		io.WriteString(w, `{"choices":[{"message":{"content":"OK"}}]}`)
	}))
	defer provider.Close()
	s := newTestServer(t)
	store, c, _, _ := aiFixture(t)
	s.cfg.AI = store
	if err := c.SetExtra(chat.AgentConfig{API: provider.URL, ModelName: "snapshot", Key: "synthetic"}); err != nil {
		t.Fatal(err)
	}
	_, revision, _ := store.repository.Read()
	cookie, csrf := enroll(t, s)
	body := map[string]any{"entry": "chat", "revision": revision}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- request(s, "POST", "/api/ai-test", body, cookie, csrf) }()
	<-entered
	expectStatus(t, request(s, "POST", "/api/ai-test", body, cookie, csrf), 429)
	if err := c.SetExtra(chat.AgentConfig{API: "http://127.0.0.1:1", ModelName: "new-model", Key: "new-synthetic"}); err != nil {
		t.Fatal(err)
	}
	close(release)
	w := <-done
	expectStatus(t, w, 200)
	var result modelProbeResult
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if !result.Success || result.Model != "snapshot" || result.Revision != revision || calls.Load() != 1 {
		t.Fatal(w.Body.String())
	}
	s.mu.Lock()
	busy := s.probeActive
	s.mu.Unlock()
	if busy {
		t.Fatal("gate not released")
	}
}

func TestModelProbeErrorsNoBodyDisclosureOrRetry(t *testing.T) {
	for _, tc := range []struct {
		name, body, category string
		code                 int
		slow                 bool
	}{
		{"http", "SYNTHETIC_SECRET private-body", "http_error", 401, false},
		{"redirect", "private-body", "http_error", 302, false},
		{"invalid", "not-json private-body", "invalid_response", 200, false},
		{"empty", `{"choices":[{"message":{"content":" "}}]}`, "invalid_response", 200, false},
		{"limit", strings.Repeat("x", (256<<10)+1), "response_limit", 200, false},
		{"timeout", "", "network_error", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if tc.slow {
					time.Sleep(80 * time.Millisecond)
				}
				w.Header().Set("Location", "/must-not-follow")
				w.WriteHeader(tc.code)
				io.WriteString(w, tc.body)
			}))
			defer provider.Close()
			client := provider.Client()
			client.Timeout = 20 * time.Millisecond
			cfg := chat.AgentConfig{Key: "SYNTHETIC_SECRET"}
			result := runModelProbe(cfg, "chat", "revision", 0, "model", provider.URL, "SYNTHETIC_SECRET", client)
			if result.Success || result.Category != tc.category || calls.Load() != 1 {
				t.Fatal(result, calls.Load())
			}
			raw, _ := json.Marshal(result)
			if bytes.Contains(raw, []byte("SYNTHETIC_SECRET")) || bytes.Contains(raw, []byte("private-body")) {
				t.Fatal("provider body disclosed")
			}
		})
	}
}

func TestModelProbeExtendsOnlyItsHTTPWriteDeadline(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(120 * time.Millisecond)
		io.WriteString(w, `{"choices":[{"message":{"content":"OK"}}]}`)
	}))
	defer provider.Close()
	s := newTestServer(t)
	store, c, _, _ := aiFixture(t)
	s.cfg.AI = store
	if err := c.SetExtra(chat.AgentConfig{API: provider.URL, ModelName: "selected", Key: "synthetic"}); err != nil {
		t.Fatal(err)
	}
	_, revision, _ := store.repository.Read()
	cookie, csrf := enroll(t, s)
	live := httptest.NewUnstartedServer(s.Handler())
	live.Config.WriteTimeout = 40 * time.Millisecond
	live.Start()
	defer live.Close()
	body, _ := json.Marshal(modelProbeRequest{Entry: "chat", Revision: revision})
	r, _ := http.NewRequest("POST", live.URL+"/api/ai-test", bytes.NewReader(body))
	r.Host = s.cfg.Addr
	r.AddCookie(cookie)
	r.Header.Set("Origin", "http://"+s.cfg.Addr)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Local-Request", "1")
	r.Header.Set("X-CSRF-Token", csrf)
	resp, err := live.Client().Do(r)
	if err != nil {
		t.Fatal("diagnostic inherited the shorter API deadline", err)
	}
	defer resp.Body.Close()
	var result modelProbeResult
	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil || !result.Success {
		t.Fatal(result, err)
	}
	if live.Config.WriteTimeout != 40*time.Millisecond {
		t.Fatal("changed every API's timeout")
	}
}
