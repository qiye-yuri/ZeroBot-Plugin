package localadmin

import (
	"strings"
	"testing"

	"github.com/FloatTech/zbputils/chat"
)

func TestTaskPhaseAPIValidationAndBinding(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentTasks{}
	s.cfg.AgentTasks = func(int64) (AgentTaskStore, error) { return f, nil }
	base := "/api/agent/tasks?botId=12345678&type=private&id=10"
	expectStatus(t, request(s, "GET", base+"&phase=model_wait", nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	for _, phase := range []string{"", "bad", "MODEL_WAIT", "model_wait&phase=finished"} {
		expectStatus(t, request(s, "GET", base+"&phase="+phase, nil, cookie, ""), 400)
	}
	read := strings.Replace(base, "/tasks?", "/tasks/read?", 1) + "&taskId=" + strings.Repeat("a", 32)
	for _, tail := range []string{"&phase=all", "&phase="} {
		expectStatus(t, request(s, "GET", read+tail, nil, cookie, ""), 400)
	}
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "taskId": strings.Repeat("a", 32), "version": "1"}
	expectStatus(t, request(s, "POST", "/api/agent/tasks/cancel?phase=all", body, cookie, csrf), 400)
	body["phase"] = "model_wait"
	expectStatus(t, request(s, "POST", "/api/agent/tasks/cancel", body, cookie, csrf), 400)
	if f.calls != 0 {
		t.Fatal("invalid request reached storage")
	}
	for _, phase := range []string{"all", "starting", "model_wait", "fallback_model_wait", "action_wait", "between_actions", "finished"} {
		w := request(s, "GET", base+"&state=cancel_requested&reason=none&phase="+phase+"&before=9007199254740993", nil, cookie, "")
		expectStatus(t, w, 200)
		if f.phase != phase || f.state != "cancel_requested" || f.reason != "none" || f.grp != -10 || f.before != 9007199254740993 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(f, w.Header())
		}
	}
	expectStatus(t, request(s, "HEAD", base+"&phase=action_wait", nil, cookie, ""), 200)
	if f.phase != "action_wait" {
		t.Fatal(f)
	}
	expectStatus(t, request(s, "GET", base, nil, cookie, ""), 200)
	if f.phase != "all" {
		t.Fatal("default changed", f)
	}
	f.err = chat.ErrAgentTaskStore
	w := request(s, "GET", base+"&phase=model_wait", nil, cookie, "")
	expectStatus(t, w, 503)
	if strings.Contains(w.Body.String(), "\"items\"") {
		t.Fatal("failed read looked empty")
	}
}
