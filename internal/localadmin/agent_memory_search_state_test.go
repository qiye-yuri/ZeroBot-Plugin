package localadmin

import (
	"strings"
	"testing"
	"time"
)

func TestMemorySearchStateAPI(t *testing.T) {
	s := newTestServer(t)
	store := &fakeAgentMemory{}
	s.cfg.AgentMemory = func(int64) (AgentMemoryStore, error) { return store, nil }
	cookie, csrf := enroll(t, s)
	path := "/api/agent/memories/search"
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "keyword": "needle", "before": "9007199254740993", "source": "admin", "state": "disabled"}
	expectStatus(t, request(s, "POST", path, body, nil, ""), 401)
	expectStatus(t, request(s, "POST", path, body, cookie, "bad"), 403)
	for _, state := range []string{"", "active", "disabled", "all"} {
		s.archiveLastSearch = time.Time{}
		body["state"] = state
		w := request(s, "POST", path, body, cookie, csrf)
		expectStatus(t, w, 200)
		want := state
		if want == "" {
			want = "active"
		}
		if store.searchState != want || store.searchSource != "admin" || store.grp != -10 || store.before != 9007199254740993 || !strings.Contains(w.Body.String(), `"state":"`+want+`"`) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(store, w.Body.String())
		}
	}
	for _, state := range []any{"unknown", "Active", 1} {
		body["state"] = state
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	}
	body["state"] = "disabled"
	for _, endpoint := range []string{"create", "update", "migration-preview", "migrate"} {
		expectStatus(t, request(s, "POST", "/api/agent/memories/"+endpoint, body, cookie, csrf), 400)
	}
	delete(body, "keyword")
	delete(body, "source")
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	body["source"] = "agent"
	s.archiveLastSearch = time.Time{}
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 200)
	store.fail = true
	s.archiveLastSearch = time.Time{}
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 503)
}
