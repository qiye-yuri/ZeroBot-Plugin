package localadmin

import (
	"strings"
	"testing"
	"time"
)

func TestMemoryRevisionTimeAPI(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentMemory{}
	s.cfg.AgentMemory = func(int64) (AgentMemoryStore, error) { return f, nil }
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "from": "2026-10-16T08:00:00.000000001+08:00", "until": "2026-10-16T00:00:00.000000003Z", "before": "9007199254740993", "source": "admin", "state": "disabled"}
	path := "/api/agent/memories/search"
	expectStatus(t, request(s, "POST", path, body, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "POST", path, body, cookie, "bad"), 403)
	for _, tc := range []struct {
		key   string
		value any
	}{{"from", true}, {"from", "bad"}, {"from", "2026-10-16T08:00:00"}, {"from", body["until"]}, {"until", "2026-10-15T00:00:00Z"}} {
		original := body[tc.key]
		body[tc.key] = tc.value
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
		body[tc.key] = original
	}
	for _, endpoint := range []string{"/api/agent/memories/create", "/api/agent/memories/update"} {
		expectStatus(t, request(s, "POST", endpoint, body, cookie, csrf), 400)
	}
	for _, endpoint := range []string{"/api/agent/memories", "/api/agent/memories/read", "/api/agent/memories/current"} {
		for _, bound := range []string{"from", "until"} {
			expectStatus(t, request(s, "GET", endpoint+"?botId=12345678&type=private&id=10&memoryId=1&revision=1&"+bound+"=", nil, cookie, ""), 400)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid request reached storage", f.calls)
	}
	w := request(s, "POST", path, body, cookie, csrf)
	expectStatus(t, w, 200)
	if f.grp != -10 || f.before != 9007199254740993 || f.searchState != "disabled" || f.searchSource != "admin" || f.searchFrom != body["from"] || f.searchUntil != body["until"] || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"from":"2026-10-16T00:00:00.000000001Z"`) {
		t.Fatal(f, w.Body.String())
	}
	delete(body, "source")
	delete(body, "state")
	delete(body, "before")
	for _, bounds := range [][2]string{{"2026-10-16T00:00:00Z", ""}, {"", "2026-10-17T00:00:00Z"}, {"", ""}} {
		body["from"], body["until"] = bounds[0], bounds[1]
		s.archiveLastSearch = time.Time{}
		code := 200
		if bounds[0] == "" && bounds[1] == "" {
			code = 400
		}
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), code)
	}
	body["from"] = "2026-10-16T00:00:00Z"
	s.archiveLastSearch = time.Time{}
	f.fail = true
	w = request(s, "POST", path, body, cookie, csrf)
	expectStatus(t, w, 503)
	if strings.Contains(w.Body.String(), `"memories"`) {
		t.Fatal("failure looked empty")
	}
}
