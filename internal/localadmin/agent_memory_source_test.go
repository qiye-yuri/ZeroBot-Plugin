package localadmin

import (
	"strings"
	"testing"
	"time"
)

func TestMemorySourceSearchAPI(t *testing.T) {
	s := newTestServer(t)
	store := &fakeAgentMemory{}
	s.cfg.AgentMemory = func(int64) (AgentMemoryStore, error) { return store, nil }
	cookie, csrf := enroll(t, s)
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "keyword": "needle", "before": "9007199254740993", "source": "admin"}
	path := "/api/agent/memories/search"
	expectStatus(t, request(s, "POST", path, body, nil, ""), 401)
	expectStatus(t, request(s, "POST", path, body, cookie, "bad"), 403)
	for _, source := range []string{"", "agent", "admin", "legacy"} {
		s.archiveLastSearch = time.Time{}
		body["source"] = source
		w := request(s, "POST", path, body, cookie, csrf)
		expectStatus(t, w, 200)
		if store.searchSource != source || store.grp != -10 || store.before != 9007199254740993 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(store, w)
		}
		if source != "" && !strings.Contains(w.Body.String(), `"source":"`+source+`"`) {
			t.Fatal(w.Body.String())
		}
	}
	for _, keyword := range []any{nil, "", " \t "} {
		if keyword == nil {
			delete(body, "keyword")
		} else {
			body["keyword"] = keyword
		}
		body["source"] = "admin"
		s.archiveLastSearch = time.Time{}
		w := request(s, "POST", path, body, cookie, csrf)
		expectStatus(t, w, 200)
		if store.keyword != "" || store.searchSource != "admin" || store.grp != -10 || store.before != 9007199254740993 {
			t.Fatal("source-only query changed", store)
		}
	}
	calls := store.calls
	for _, source := range []string{"", "unknown"} {
		body["source"] = source
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	}
	if store.calls != calls {
		t.Fatal("invalid source-only query reached storage")
	}
	body["source"] = "unknown"
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	body["source"] = "admin"
	expectStatus(t, request(s, "POST", "/api/agent/memories/create", body, cookie, csrf), 400)
	expectStatus(t, request(s, "GET", "/api/agent/memories?botId=12345678&type=private&id=10&source=admin", nil, cookie, ""), 400)
}
