package localadmin

import (
	"strings"
	"testing"

	"github.com/FloatTech/zbputils/chat"
)

func TestTaskReasonAPIValidationAndBinding(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentTasks{}
	s.cfg.AgentTasks = func(bot int64) (AgentTaskStore, error) {
		if bot != 12345678 {
			t.Fatal(bot)
		}
		return f, nil
	}
	base := "/api/agent/tasks?botId=12345678&type=private&id=10"
	expectStatus(t, request(s, "GET", base+"&reason=storage_error", nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	for _, reason := range []string{"", "bad", "MODEL_REQUEST_FAILED", "model_output_invalid&reason=storage_error"} {
		expectStatus(t, request(s, "GET", base+"&reason="+reason, nil, cookie, ""), 400)
	}
	read := strings.Replace(base, "/tasks?", "/tasks/read?", 1) + "&taskId=" + strings.Repeat("a", 32)
	for _, tail := range []string{"&reason=all", "&reason="} {
		expectStatus(t, request(s, "GET", read+tail, nil, cookie, ""), 400)
	}
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "taskId": strings.Repeat("a", 32), "version": "1"}
	expectStatus(t, request(s, "POST", "/api/agent/tasks/cancel?reason=all", body, cookie, csrf), 400)
	body["reason"] = "storage_error"
	expectStatus(t, request(s, "POST", "/api/agent/tasks/cancel", body, cookie, csrf), 400)
	expectStatus(t, request(s, "GET", strings.Replace(base, "12345678", "999", 1)+"&reason=other", nil, cookie, ""), 404)
	if f.calls != 0 {
		t.Fatal("invalid request reached storage")
	}
	for _, reason := range []string{"all", "none", "other", "model_request_failed", "model_output_invalid", "model_output_limit", "storage_error", "settings_changed", "repeated_action", "process_interrupted", "execution_error"} {
		w := request(s, "GET", base+"&state=failed&reason="+reason+"&before=9007199254740993", nil, cookie, "")
		expectStatus(t, w, 200)
		if f.state != "failed" || f.reason != reason || f.grp != -10 || f.before != 9007199254740993 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(f, w.Header())
		}
	}
	expectStatus(t, request(s, "GET", base, nil, cookie, ""), 200)
	if f.reason != "all" || f.state != "all" || f.before != 0 {
		t.Fatal("defaults changed", f)
	}
	f.version = "unchanged"
	expectStatus(t, request(s, "HEAD", base+"&reason=none", nil, cookie, ""), 200)
	if f.reason != "none" || f.version != "unchanged" {
		t.Fatal("HEAD mutated task", f)
	}
	f.err = chat.ErrAgentTaskStore
	w := request(s, "GET", base+"&reason=other", nil, cookie, "")
	expectStatus(t, w, 503)
	if strings.Contains(w.Body.String(), "\"items\"") {
		t.Fatal("failed read looked empty")
	}
}
