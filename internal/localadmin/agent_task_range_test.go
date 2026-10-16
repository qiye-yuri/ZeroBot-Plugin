package localadmin

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/FloatTech/zbputils/chat"
)

func TestTaskStartRangeAPI(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentTasks{}
	s.cfg.AgentTasks = func(int64) (AgentTaskStore, error) { return f, nil }
	base := "/api/agent/tasks?botId=12345678&type=private&id=10"
	expectStatus(t, request(s, "GET", base+"&from=bad", nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	for _, tail := range []string{"&from=", "&until=", "&from=bad", "&from=2026-10-16T00:00:00", "&from=0001-01-01T00:00:00Z", "&from=2026-10-16T00:00:00Z&until=2026-10-16T00:00:00Z", "&from=2026-10-17T00:00:00Z&until=2026-10-16T00:00:00Z", "&from=2026-10-16T00:00:00Z&from=2026-10-17T00:00:00Z", "&until=2026-10-16T00:00:00Z&until=2026-10-17T00:00:00Z"} {
		expectStatus(t, request(s, "GET", base+tail, nil, cookie, ""), 400)
	}
	for _, name := range []string{"from", "until"} {
		read := strings.Replace(base, "/tasks?", "/tasks/read?", 1) + "&taskId=" + strings.Repeat("a", 32) + "&" + name + "=2026-10-16T00:00:00Z"
		expectStatus(t, request(s, "GET", read, nil, cookie, ""), 400)
		body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "taskId": strings.Repeat("a", 32), "version": "1"}
		expectStatus(t, request(s, "POST", "/api/agent/tasks/cancel?"+name+"=2026-10-16T00:00:00Z", body, cookie, csrf), 400)
		body[name] = "2026-10-16T00:00:00Z"
		expectStatus(t, request(s, "POST", "/api/agent/tasks/cancel", body, cookie, csrf), 400)
	}
	if f.calls != 0 {
		t.Fatal("invalid request reached storage", f)
	}
	from, until := "2026-10-16T08:00:00.000000001+08:00", "2026-10-16T00:00:00.000000002Z"
	w := request(s, "GET", base+"&state=failed&reason=execution_error&phase=finished&before=9007199254740993&from="+url.QueryEscape(from)+"&until="+url.QueryEscape(until), nil, cookie, "")
	expectStatus(t, w, 200)
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if f.grp != -10 || f.before != 9007199254740993 || f.state != "failed" || f.reason != "execution_error" || f.phase != "finished" || f.from != "2026-10-16T00:00:00.000000001Z" || f.until != until || response["from"] != f.from || response["until"] != until || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(f, response, w.Header())
	}
	expectStatus(t, request(s, "HEAD", base+"&until="+until, nil, cookie, ""), 200)
	if f.from != "" || f.until != until {
		t.Fatal(f)
	}
	expectStatus(t, request(s, "GET", base, nil, cookie, ""), 200)
	if f.from != "" || f.until != "" {
		t.Fatal("default retained time range", f)
	}
	f.err = chat.ErrAgentTaskStore
	w = request(s, "GET", base+"&from="+url.QueryEscape(from), nil, cookie, "")
	expectStatus(t, w, 503)
	if strings.Contains(w.Body.String(), "\"items\"") {
		t.Fatal("failed read looked empty")
	}
}
