package localadmin

import (
	"strings"
	"testing"

	"github.com/FloatTech/zbputils/chat"
)

func TestTaskOutcomeAPIValidationAndBinding(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentTasks{}
	s.cfg.AgentTasks = func(int64) (AgentTaskStore, error) { return f, nil }
	base := "/api/agent/tasks?botId=12345678&type=private&id=10"
	expectStatus(t, request(s, "GET", base+"&outcome=unknown", nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	for _, outcome := range []string{"", "bad", "UNKNOWN", "unknown&outcome=ok"} {
		expectStatus(t, request(s, "GET", base+"&outcome="+outcome, nil, cookie, ""), 400)
	}
	read := strings.Replace(base, "/tasks?", "/tasks/read?", 1) + "&taskId=" + strings.Repeat("a", 32)
	for _, tail := range []string{"&outcome=all", "&outcome="} {
		expectStatus(t, request(s, "GET", read+tail, nil, cookie, ""), 400)
	}
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "taskId": strings.Repeat("a", 32), "version": "1"}
	expectStatus(t, request(s, "POST", "/api/agent/tasks/cancel?outcome=all", body, cookie, csrf), 400)
	body["outcome"] = "unknown"
	expectStatus(t, request(s, "POST", "/api/agent/tasks/cancel", body, cookie, csrf), 400)
	if f.calls != 0 {
		t.Fatal("invalid request reached store", f)
	}
	for _, outcome := range []string{"all", "none", "other", "in_flight", "ok", "rejected", "unknown", "local_error"} {
		w := request(s, "GET", base+"&state=failed&reason=execution_error&phase=finished&outcome="+outcome+"&before=9007199254740993&from=2026-10-16T00:00:00Z", nil, cookie, "")
		expectStatus(t, w, 200)
		if f.outcome != outcome || f.state != "failed" || f.reason != "execution_error" || f.phase != "finished" || f.before != 9007199254740993 || f.grp != -10 || f.from != "2026-10-16T00:00:00Z" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(f, w.Header())
		}
	}
	expectStatus(t, request(s, "HEAD", base+"&outcome=in_flight", nil, cookie, ""), 200)
	if f.outcome != "in_flight" {
		t.Fatal(f)
	}
	expectStatus(t, request(s, "GET", base, nil, cookie, ""), 200)
	if f.outcome != "all" {
		t.Fatal("default changed", f)
	}
	f.err = chat.ErrAgentTaskStore
	w := request(s, "GET", base+"&outcome=unknown", nil, cookie, "")
	expectStatus(t, w, 503)
	if strings.Contains(w.Body.String(), "\"items\"") {
		t.Fatal("failed read looked empty")
	}
}
