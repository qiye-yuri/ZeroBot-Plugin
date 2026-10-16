package localadmin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/FloatTech/zbputils/chat"
)

func TestArchiveEventTypeSearchAPI(t *testing.T) {
	s := newTestServer(t)
	calls := 0
	var last chat.ArchiveQuery
	s.cfg.AgentArchive = func(_ context.Context, bot int64, q chat.ArchiveQuery) (chat.ArchivePage, error) {
		calls++
		last = q
		if bot != 12345678 {
			t.Fatal(bot)
		}
		return chat.ArchivePage{Available: true, Entries: []chat.ArchiveEntry{}}, nil
	}
	cookie, csrf := enroll(t, s)
	path := "/api/agent/history/search"
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "eventType": "notice", "before": "9007199254740993", "archived": true, "from": "2026-10-15T08:00:00+08:00"}
	expectStatus(t, request(s, "POST", path, body, nil, ""), 401)
	expectStatus(t, request(s, "POST", path, body, cookie, "bad"), 403)
	for _, eventType := range []string{"message", "notice", "request", ""} {
		s.archiveLastSearch = time.Time{}
		body["eventType"] = eventType
		w := request(s, "POST", path, body, cookie, csrf)
		expectStatus(t, w, 200)
		if last.SearchEventType != eventType || last.Conversation != -10 || last.Before != 9007199254740993 || !last.IncludeResetHistory || last.SearchFrom.IsZero() || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(last, w)
		}
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if value, present := result["eventType"]; (eventType != "" && value != eventType) || (eventType == "" && present) {
			t.Fatal(result)
		}
	}
	for _, invalid := range []any{"Message", "event", "response", 1, true} {
		body["eventType"] = invalid
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	}
	for _, query := range []string{"/api/agent/history?botId=12345678&type=private&id=10&eventType=notice", "/api/agent/history?botId=12345678&type=private&id=10&seq=1&eventType=", "/api/agent/conversations?botId=12345678&eventType=message"} {
		expectStatus(t, request(s, "GET", query, nil, cookie, ""), 400)
	}
	before := calls
	delete(body, "from")
	body["eventType"] = ""
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	if calls != before {
		t.Fatal("empty query reached archive")
	}
	body["eventType"] = "request"
	s.archiveLastSearch = time.Time{}
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 200)
	if last.Keyword != "" || last.SearchEventType != "request" {
		t.Fatal("event-only query lost conditions", last)
	}
}
