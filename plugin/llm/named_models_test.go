package llm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/FloatTech/zbputils/chat"
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
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			cfg := chat.AgentConfig{API: "http://127.0.0.1:1", ModelName: "wrong-legacy", ModelProfiles: []chat.ModelProfile{{ID: "selected", Name: "配置", Type: tc.kind, Model: "chosen-profile", API: server.URL, Key: "synthetic-selected-key"}}, ModelRoutes: &chat.ModelRoutes{Chat: "selected"}}
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
