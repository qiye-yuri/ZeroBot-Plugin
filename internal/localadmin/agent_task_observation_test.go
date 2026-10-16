package localadmin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FloatTech/zbputils/chat"
)

func TestTaskResponsesUseServerObservationTime(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentTasks{}
	s.cfg.AgentTasks = func(int64) (AgentTaskStore, error) { return f, nil }
	clock := time.Date(2026, 10, 15, 20, 10, 2, 123456789, time.FixedZone("fixture", 8*3600))
	s.now = func() time.Time { return clock }
	cookie, csrf := enroll(t, s)
	base := "/api/agent/tasks?botId=12345678&type=private&id=10"
	id := strings.Repeat("a", 32)
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "taskId": id, "version": "9007199254740993"}
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{"GET", base, nil},
		{"GET", base + "&state=running&before=9007199254740993", nil},
		{"GET", "/api/agent/tasks/read?botId=12345678&type=private&id=10&taskId=" + id, nil},
		{"POST", "/api/agent/tasks/cancel", body},
	} {
		for i := 0; i < 2; i++ {
			clock = clock.Add(time.Second)
			w := request(s, tc.method, tc.path, tc.body, cookie, csrf)
			expectStatus(t, w, 200)
			var value map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
				t.Fatal(err)
			}
			if value["observedAt"] != clock.UTC().Format(time.RFC3339Nano) || w.Header().Get("Cache-Control") != "no-store" || f.grp != -10 {
				t.Fatal(tc, value, f)
			}
			if rows, ok := value["items"].([]any); ok {
				if len(rows) != 1 || rows[0].(map[string]any)["version"] != "9007199254740993" {
					t.Fatal("list task changed", value)
				}
				if _, saved := rows[0].(map[string]any)["observedAt"]; saved {
					t.Fatal("observation attached to stored item")
				}
			} else if value["taskId"] != id || value["version"] != "9007199254740993" {
				t.Fatal("task identity or version changed", value)
			}
		}
	}
	if f.calls != 8 {
		t.Fatal("observation added storage operations", f.calls)
	}
	// Failed reads do not masquerade as a valid observed task.
	f.err = chat.ErrAgentTaskStore
	w := request(s, "GET", base, nil, cookie, csrf)
	expectStatus(t, w, 503)
	if strings.Contains(w.Body.String(), "observedAt") {
		t.Fatal(w.Body.String())
	}
}
