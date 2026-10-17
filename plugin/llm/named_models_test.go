package llm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FloatTech/zbputils/chat"
	"github.com/FloatTech/zbputils/modelconfig"
)

func TestNamedModelSelectionReachesEachProtocol(t *testing.T) {
	for _, tc := range []struct {
		kind chat.ModelType
		body string
	}{
		{0, `{"choices":[{"message":{"role":"assistant","content":"selected result"}}]}`},
		{1, `{"message":{"role":"assistant","content":"selected result"}}`},
		{2, `{"candidates":[{"content":{"parts":[{"text":"selected result"}]}}]}`},
	} {
		t.Run(tc.kind.String(), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(body)+r.URL.String(), "chosen-profile") {
					t.Error("request did not use selected model")
				}
				var payload map[string]any
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatal(err)
				}
				if tc.kind == 2 {
					payload = payload["generationConfig"].(map[string]any)
					if payload["maxOutputTokens"] != float64(777) || payload["topP"] != .35 {
						t.Error("GenAI parameters", payload)
					}
				} else {
					if payload["max_tokens"] != float64(777) || payload["top_p"] != .35 {
						t.Error("model parameters", payload)
					}
					if tc.kind == 0 && payload["reasoning_effort"] != "low" {
						t.Error("reasoning override lost")
					}
				}
				if payload["temperature"] != .7 {
					t.Error("conversation temperature changed")
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			cfg := chat.AgentConfig{API: "http://127.0.0.1:1", ModelName: "wrong-legacy", ModelProfiles: []chat.ModelProfile{{ID: "selected", Name: "配置", Type: tc.kind, Model: "chosen-profile", API: server.URL, Key: "synthetic-selected-key"}}, ModelRoutes: &chat.ModelRoutes{Chat: "selected"}}
			max, top, reasoning := uint(777), float32(.35), "low"
			cfg.MaxN, cfg.TopP, cfg.ReasoningEffort = 2048, .8, "high"
			cfg.ModelProfiles[0].Parameters = &modelconfig.Parameters{MaxN: &max, TopP: &top, ReasoningEffort: &reasoning}
			selected, err := cfg.ResolveModels()
			if err != nil {
				t.Fatal(err)
			}
			result, err := requestLLM(selected, "hello", 0.7)
			if err != nil || result != "selected result" || calls != 1 {
				t.Fatal("named protocol request failed", result, calls, err)
			}
		})
	}
}
