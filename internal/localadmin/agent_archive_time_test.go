package localadmin

import (
	"context"
	"testing"
	"time"

	"github.com/FloatTech/zbputils/chat"
)

func TestArchiveSearchTimeAuthorizationAndValidation(t *testing.T) {
	s := newTestServer(t)
	calls := 0
	var last chat.ArchiveQuery
	s.cfg.AgentArchive = func(_ context.Context, _ int64, q chat.ArchiveQuery) (chat.ArchivePage, error) {
		calls++
		last = q
		return chat.ArchivePage{Available: true, Entries: []chat.ArchiveEntry{}}, nil
	}
	cookie, csrf := enroll(t, s)
	path := "/api/agent/history/search"
	body := map[string]any{"botId": "12345678", "type": "group", "id": "10", "keyword": "needle", "from": "2026-10-15T08:00:00.000000001+08:00", "until": "2026-10-15T00:00:00.000000002Z", "before": "9007199254740993"}
	expectStatus(t, request(s, "POST", path, body, nil, ""), 401)
	expectStatus(t, request(s, "POST", path, body, cookie, "wrong"), 403)
	for _, bad := range []any{"2026-10-15", "2026-10-15T00:00:00", "bad", 123, "0001-01-01T00:00:00Z", "2026-10-16T00:00:00Z", "2026-10-15T00:00:00.000000002Z"} {
		body["from"] = bad
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	}
	for _, suffix := range []string{"from=", "until=2026-10-15T00:00:00Z"} {
		expectStatus(t, request(s, "GET", "/api/agent/history?botId=12345678&type=group&id=10&"+suffix, nil, cookie, ""), 400)
		expectStatus(t, request(s, "GET", "/api/agent/conversations?botId=12345678&"+suffix, nil, cookie, ""), 400)
	}
	if calls != 0 {
		t.Fatal("invalid range reached store")
	}
	body["from"] = "2026-10-15T08:00:00.000000001+08:00"
	w := request(s, "POST", path, body, cookie, csrf)
	expectStatus(t, w, 200)
	want, _ := time.Parse(time.RFC3339Nano, "2026-10-15T00:00:00.000000001Z")
	if !last.SearchFrom.Equal(want) || last.SearchFrom.Location() != time.UTC || last.SearchUntil.Sub(last.SearchFrom) != time.Nanosecond || last.Before != 9007199254740993 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(last)
	}
	for _, keyword := range []any{nil, "", "  "} {
		s.archiveLastSearch = time.Time{}
		if keyword == nil {
			delete(body, "keyword")
		} else {
			body["keyword"] = keyword
		}
		w = request(s, "POST", path, body, cookie, csrf)
		expectStatus(t, w, 200)
		if last.Keyword != "" || !last.SearchFrom.Equal(want) || last.Before != 9007199254740993 {
			t.Fatal(last)
		}
	}
	body["keyword"] = "needle"
	s.archiveLastSearch = time.Time{}
	body["from"] = ""
	delete(body, "until")
	delete(body, "before")
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 200)
	if !last.SearchFrom.IsZero() || !last.SearchUntil.IsZero() || last.Before != 0 {
		t.Fatal("range retained", last)
	}
}
