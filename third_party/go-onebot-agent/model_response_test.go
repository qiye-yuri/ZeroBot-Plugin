package goba

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
)

func responseEnvelope(content any) string {
	raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}}}})
	return string(raw)
}

func TestAgentModelTextResponses(t *testing.T) {
	action := `{"action":"send_group_msg","params":{"group_id":9007199254740993,"message":"你好"}}` + "\n" + `{"action":"end_action","params":{}}`
	cut := strings.Index(action, "9007") + 3 // Split a number, never add separators.
	openAI := func() model.Protocol { return model.NewOpenAI("synthetic", "", 0, 1, 128) }
	genAI := func() model.Protocol { return model.NewGenAI("synthetic", 0, 1, 128) }
	ollama := func() model.Protocol { return model.NewOLLaMA("synthetic", "", 0, 1, 128) }
	gen, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": action[:cut]}, map[string]any{"text": action[cut:]}}}}}})
	oll, _ := json.Marshal(map[string]any{"message": map[string]any{"role": "assistant", "content": action}})
	for _, tc := range []struct {
		name, body     string
		protocol       func() model.Protocol
		invalid, empty bool
	}{
		{"openai-text-parts", responseEnvelope([]any{map[string]any{"type": "text", "text": action[:cut]}, map[string]any{"type": "text", "text": action[cut:]}}), openAI, false, false},
		{"genai-text-parts", string(gen), genAI, false, false},
		{"ollama-text", string(oll), ollama, false, false},
		{"missing-choices", `{}`, openAI, true, false},
		{"error-envelope", `{"error":{"message":"PRIVATE_RESPONSE"}}`, openAI, true, false},
		{"empty-choices", `{"choices":[]}`, openAI, true, false},
		{"missing-content", `{"choices":[{"message":{"role":"assistant"}}]}`, openAI, true, false},
		{"null-content", responseEnvelope(nil), openAI, true, false},
		{"number-content", responseEnvelope(42), openAI, true, false},
		{"empty-parts", responseEnvelope([]any{}), openAI, true, false},
		{"unknown-part", responseEnvelope([]any{map[string]any{"type": "private-unknown-type", "text": "PRIVATE_RESPONSE"}}), openAI, true, false},
		{"mixed-parts", responseEnvelope([]any{map[string]any{"type": "text", "text": `{"action":"save_memory","params":{"text":"MUST_NOT_SAVE"}}`}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://synthetic.invalid/private"}}}), openAI, true, false},
		{"genai-missing", `{}`, genAI, true, false},
		{"genai-conversion-panic", `{"candidates":[{"content":{"parts":[{}]}}]}`, genAI, true, false},
		{"ollama-missing", `{}`, ollama, true, false},
		{"empty-string-compatible", responseEnvelope(""), openAI, false, true},
		{"empty-text-part-compatible", responseEnvelope([]any{map[string]any{"type": "text", "text": ""}}), openAI, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, tc.body) }))
			defer server.Close()
			history, memory := &preparationHistory{}, &actionLimitMemory{}
			ag := NewAgent(&Config{}, 16, 8, time.Hour, "", memory, !tc.invalid, !tc.invalid)
			ag.SetHistoryStore(history)
			api := deepinfra.NewAPI(server.URL, "")
			api.SetHTTPClient(server.Client())
			reqs, err := ag.GetAction(api, tc.protocol(), 10, PermRoleOwner, 0, false)
			if tc.invalid {
				if !errors.Is(err, ErrModelRequestFailed) || err.Error() != ErrModelRequestFailed.Error() || reqs != nil || history.appends.Load() != 0 || memory.saved.Load() != 0 {
					t.Fatal("invalid content escaped response boundary", err, len(reqs))
				}
			} else if tc.empty {
				if err != nil || len(reqs) != 0 {
					t.Fatal("explicit empty text compatibility changed", err)
				}
			} else if !errors.Is(err, io.EOF) || len(reqs) != 1 || reqs[0].Params["group_id"] != json.Number("9007199254740993") {
				t.Fatal("text parts were not joined exactly", err, reqs)
			}
		})
	}
}

func TestAgentResponseTextCapacity(t *testing.T) {
	for _, extra := range []int{0, 1} {
		text, err := agentResponseText(model.Contents{model.NewContentText(strings.Repeat("字", (agentActionOutputBytes-1)/3)), model.NewContentText(strings.Repeat("x", agentActionOutputBytes-3*((agentActionOutputBytes-1)/3)+extra))})
		if extra == 0 {
			if err != nil || len(text) != agentActionOutputBytes {
				t.Fatal("exact text limit rejected", err, len(text))
			}
		} else if !errors.Is(err, ErrActionOutputLimit) || text != "" {
			t.Fatal("combined overflow partially returned", err, len(text))
		}
	}
}

func TestAgentProtocolReuseDoesNotReplayOldResponse(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			io.WriteString(w, responseEnvelope(`{"action":"end_action"}`))
		} else {
			io.WriteString(w, `{}`)
		}
	}))
	defer server.Close()
	ag := NewAgent(&Config{}, 16, 8, time.Hour, "", &actionLimitMemory{}, true, true)
	history := &preparationHistory{}
	ag.SetHistoryStore(history)
	api := deepinfra.NewAPI(server.URL, "")
	api.SetHTTPClient(server.Client())
	p := model.NewOpenAI("synthetic", "", 0, 1, 128)
	if _, err := ag.GetAction(api, p, 10, PermRoleOwner, 0, false); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if reqs, err := ag.GetAction(api, p, 10, PermRoleOwner, 1, false); !errors.Is(err, ErrModelRequestFailed) || reqs != nil || history.appends.Load() != 1 {
		t.Fatal("stale response replayed", err, len(reqs))
	}
	if len(p.Choices) != 0 || len(p.Messages) != 0 || calls.Load() != 2 {
		t.Fatal("request polluted shared protocol", len(p.Choices), len(p.Messages), calls.Load())
	}
}

func TestAgentResponseOverflowDoesNotSaveTextPrefix(t *testing.T) {
	body := responseEnvelope([]any{map[string]any{"type": "text", "text": `{"action":"save_memory","params":{"text":"MUST_NOT_SAVE"}}`}, map[string]any{"type": "text", "text": strings.Repeat("x", agentActionOutputBytes)}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }))
	defer server.Close()
	history, memory := &preparationHistory{}, &actionLimitMemory{}
	ag := NewAgent(&Config{}, 16, 8, time.Hour, "", memory, false, false)
	ag.SetHistoryStore(history)
	api := deepinfra.NewAPI(server.URL, "")
	api.SetHTTPClient(server.Client())
	reqs, err := ag.GetAction(api, model.NewOpenAI("synthetic", "", 0, 1, 128), 10, PermRoleOwner, 0, false)
	if !errors.Is(err, ErrActionOutputLimit) || reqs != nil || history.appends.Load() != 0 || memory.saved.Load() != 0 {
		t.Fatal("fragment overflow saved prefix", err, len(reqs))
	}
}
