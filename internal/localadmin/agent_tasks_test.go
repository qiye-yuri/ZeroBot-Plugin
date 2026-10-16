package localadmin

import (
	"context"
	"strings"
	"testing"

	"github.com/FloatTech/zbputils/chat"
)

type fakeAgentTasks struct {
	calls       int
	grp         int64
	before      int64
	id, version string
	err         error
	state       string
	reason      string
	phase       string
	outcome     string
	from, until string
}

func (f *fakeAgentTasks) PageMatching(ctx context.Context, g, before int64, filter chat.TaskPageFilter) (chat.AgentTaskPage, error) {
	f.outcome = filter.Outcome
	return f.PageInRange(ctx, g, before, filter.State, filter.Reason, filter.Phase, filter.From, filter.Until)
}

func (f *fakeAgentTasks) PageInRange(ctx context.Context, g, before int64, state, reason, phase, from, until string) (chat.AgentTaskPage, error) {
	f.from, f.until = from, until
	return f.PageWithFilters(ctx, g, before, state, reason, phase)
}

func (f *fakeAgentTasks) PageWithFilters(ctx context.Context, g, before int64, state, reason, phase string) (chat.AgentTaskPage, error) {
	f.phase = phase
	return f.PageWithReason(ctx, g, before, state, reason)
}

func (f *fakeAgentTasks) PageWithReason(ctx context.Context, g, before int64, state, reason string) (chat.AgentTaskPage, error) {
	f.reason = reason
	return f.PageFiltered(ctx, g, before, state)
}

func (f *fakeAgentTasks) PageFiltered(ctx context.Context, g, before int64, state string) (chat.AgentTaskPage, error) {
	f.state = state
	return f.Page(ctx, g, before)
}

func TestAgentTaskStateFilterAPI(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentTasks{}
	s.cfg.AgentTasks = func(int64) (AgentTaskStore, error) { return f, nil }
	cookie, _ := enroll(t, s)
	base := "/api/agent/tasks?botId=12345678&type=private&id=10"
	for _, state := range []string{"all", "running", "cancel_requested", "cancelled", "completed", "failed", "interrupted"} {
		expectStatus(t, request(s, "GET", base+"&state="+state+"&before=9007199254740993", nil, cookie, ""), 200)
		if f.state != state || f.grp != -10 || f.before != 9007199254740993 {
			t.Fatal("filter scope or cursor changed")
		}
	}
	calls := f.calls
	expectStatus(t, request(s, "GET", base+"&state=bad", nil, cookie, ""), 400)
	if calls != f.calls {
		t.Fatal("invalid filter reached store")
	}
	expectStatus(t, request(s, "GET", base, nil, cookie, ""), 200)
	if f.state != "all" {
		t.Fatal("default changed")
	}
}

func (f *fakeAgentTasks) Page(_ context.Context, g, before int64) (chat.AgentTaskPage, error) {
	f.calls++
	f.grp = g
	f.before = before
	memorySources := 3
	return chat.AgentTaskPage{Items: []chat.AgentTaskInfo{{ID: strings.Repeat("a", 32), Version: "9007199254740993", Status: "running", Persisted: true, Context: chat.ContextUsage{Available: true, MemorySources: &memorySources}, Memory: chat.MemoryUsage{Available: true, Mode: "managed", Records: 7, Bytes: 27400, BudgetBytes: 30000, Limit: 100, Clipped: true, Reason: "byte_limit"}}}, Limit: 50, More: true, Next: "9007199254740993"}, f.err
}
func (f *fakeAgentTasks) Cancel(_ context.Context, g int64, id, version string) (chat.AgentTaskInfo, error) {
	f.calls++
	f.grp = g
	f.id = id
	f.version = version
	return chat.AgentTaskInfo{ID: id, Version: version, Status: "cancel_requested", Persisted: true}, f.err
}
func (f *fakeAgentTasks) Read(_ context.Context, g int64, id string) (chat.AgentTaskInfo, error) {
	f.calls++
	f.grp, f.id = g, id
	return chat.AgentTaskInfo{ID: id, Version: "9007199254740993", Status: "completed", Persisted: true, PhaseStartedAt: "2026-10-14T00:00:02.123456789Z", CancelRequestedAt: "2026-10-14T00:00:01.987654321Z", Results: chat.TaskResults{Available: true, OK: 2, Rejected: 1, Unknown: 3, LocalErrors: 4}, Queries: chat.TaskQueries{Available: true, Used: 1, Limit: 3, ReadMemory: 1, Successful: 1, Empty: 1}}, f.err
}

func TestAgentTaskReadAPI(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentTasks{}
	s.cfg.AgentTasks = func(bot int64) (AgentTaskStore, error) {
		if bot != 12345678 {
			t.Fatal(bot)
		}
		return f, nil
	}
	base := "/api/agent/tasks/read?botId=12345678&type=private&id=10&taskId=" + strings.Repeat("a", 32)
	expectStatus(t, request(s, "GET", base, nil, nil, ""), 401)
	cookie, _ := enroll(t, s)
	expectStatus(t, request(s, "GET", strings.Replace(base, "12345678", "999", 1), nil, cookie, ""), 404)
	for _, tail := range []string{"&state=all", "&state=", "&before=1", "&before=", "&version=1"} {
		expectStatus(t, request(s, "GET", base+tail, nil, cookie, ""), 400)
	}
	for _, bad := range []string{"", strings.Repeat("A", 32), strings.Repeat("g", 32), "../bad"} {
		expectStatus(t, request(s, "GET", strings.Replace(base, strings.Repeat("a", 32), bad, 1), nil, cookie, ""), 400)
	}
	expectStatus(t, request(s, "GET", strings.Replace(base, "/tasks/read", "/tasks", 1), nil, cookie, ""), 400)
	if f.calls != 0 {
		t.Fatal("invalid or unauthenticated read reached store")
	}
	w := request(s, "GET", base, nil, cookie, "")
	expectStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), `"results":{"available":true,"ok":2,"rejected":1,"unknown":3,"localErrors":4}`) {
		t.Fatal("settled result counts lost in exact task response", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"queries":{"available":true,"used":1,"limit":3,"searchHistory":0,"readHistory":0,"searchMemory":0,"readMemory":1,"successful":1,"empty":1,"partial":0,"errors":0,"denied":0}`) {
		t.Fatal("query usage lost in exact task response", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"phaseStartedAt":"2026-10-14T00:00:02.123456789Z"`) || !strings.Contains(w.Body.String(), `"cancelRequestedAt":"2026-10-14T00:00:01.987654321Z"`) {
		t.Fatal("timing fields lost in exact task API", w.Body.String())
	}
	if f.grp != -10 || f.id != strings.Repeat("a", 32) || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"version":"9007199254740993"`) {
		t.Fatal(w.Body.String(), f)
	}
	expectStatus(t, request(s, "GET", strings.Replace(base, "private", "group", 1), nil, cookie, ""), 200)
	if f.grp != 10 {
		t.Fatal("group scope lost")
	}
	f.version = "unchanged"
	expectStatus(t, request(s, "HEAD", base, nil, cookie, ""), 200)
	if f.version != "unchanged" {
		t.Fatal("HEAD dispatched a cancellation")
	}
	for _, tc := range []struct {
		e      error
		status int
	}{{chat.ErrAgentTaskMissing, 404}, {chat.ErrAgentTaskStore, 503}} {
		f.err = tc.e
		expectStatus(t, request(s, "GET", base, nil, cookie, ""), tc.status)
	}
}
func TestAgentTasksAuthenticationScopeVersionsAndFailures(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentTasks{}
	opens := 0
	s.cfg.AgentTasks = func(bot int64) (AgentTaskStore, error) {
		opens++
		if bot != 12345678 {
			t.Fatal("wrong bot")
		}
		return f, nil
	}
	base := "/api/agent/tasks?botId=12345678&type=private&id=10"
	expectStatus(t, request(s, "GET", base, nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "GET", strings.Replace(base, "12345678", "999", 1), nil, cookie, ""), 404)
	expectStatus(t, request(s, "GET", strings.Replace(base, "private", "invalid", 1), nil, cookie, ""), 400)
	if opens != 0 {
		t.Fatal("unauthorized storage access")
	}
	w := request(s, "GET", base, nil, cookie, "")
	expectStatus(t, w, 200)
	if !strings.Contains(w.Body.String(), `"memorySources":3`) {
		t.Fatal("memory context count missing")
	}
	if !strings.Contains(w.Body.String(), `"memory":{"available":true,"mode":"managed","records":7,"bytes":27400,"budgetBytes":30000,"limit":100,"clipped":true,"reason":"byte_limit"}`) {
		t.Fatal("memory summary missing", w.Body.String())
	}
	if f.grp != -10 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"version":"9007199254740993"`) {
		t.Fatal(w.Body.String(), f.grp, w.Header())
	}
	w = request(s, "GET", base+"&before=9007199254740993", nil, cookie, "")
	expectStatus(t, w, 200)
	if f.before != 9007199254740993 || !strings.Contains(w.Body.String(), `"next":"9007199254740993"`) {
		t.Fatal("pagination precision lost", w.Body.String())
	}
	for _, value := range []string{"", "0", "-1", "01", "1.5", "9223372036854775808"} {
		before := f.calls
		expectStatus(t, request(s, "GET", base+"&before="+value, nil, cookie, ""), 400)
		if f.calls != before {
			t.Fatal("invalid cursor queried storage")
		}
	}
	body := map[string]any{"botId": "12345678", "type": "group", "id": "10", "taskId": strings.Repeat("a", 32), "version": "9007199254740993"}
	before := f.calls
	expectStatus(t, request(s, "POST", "/api/agent/tasks/cancel", body, cookie, "wrong"), 403)
	if before != f.calls {
		t.Fatal("CSRF executed")
	}
	body["taskId"] = "../bad"
	expectStatus(t, request(s, "POST", "/api/agent/tasks/cancel", body, cookie, csrf), 400)
	body["taskId"] = strings.Repeat("b", 32)
	expectStatus(t, request(s, "POST", "/api/agent/tasks/cancel", body, cookie, csrf), 200)
	if f.grp != 10 || f.id != body["taskId"] || f.version != body["version"] {
		t.Fatal(f)
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{chat.ErrAgentTaskConflict, 409}, {chat.ErrAgentTaskMissing, 404}, {chat.ErrAgentTaskStore, 503}} {
		f.err = tc.err
		expectStatus(t, request(s, "POST", "/api/agent/tasks/cancel", body, cookie, csrf), tc.status)
	}
}
