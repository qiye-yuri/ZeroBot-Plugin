package localadmin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/FloatTech/zbputils/chat"
)

func TestArchiveSenderSearchParametersAndScope(t *testing.T) {
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
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "keyword": "needle", "sender": "9007199254740993", "before": "9223372036854775807", "from": "2026-10-15T08:00:00+08:00", "archived": true}
	expectStatus(t, request(s, "POST", path, body, nil, ""), 401)
	expectStatus(t, request(s, "POST", path, body, cookie, "bad"), 403)
	for _, value := range []any{nil, 123, true, []string{"123"}, "0", "01", "-1", "1.5", "9223372036854775808"} {
		body["sender"] = value
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	}
	for _, path := range []string{"/api/agent/history?botId=12345678&type=group&id=10&sender=123", "/api/agent/history?botId=12345678&type=group&id=10&seq=1&sender=", "/api/agent/conversations?botId=12345678&sender=123"} {
		expectStatus(t, request(s, "GET", path, nil, cookie, ""), 400)
	}
	if calls != 0 {
		t.Fatal("invalid sender read storage")
	}
	body["sender"] = "9007199254740993"
	w := request(s, "POST", path, body, cookie, csrf)
	expectStatus(t, w, 200)
	if last.SearchSender != 9007199254740993 || last.Conversation != -10 || last.Before != 9223372036854775807 || !last.IncludeResetHistory || !last.Search || last.SearchFrom.Format(time.RFC3339Nano) != "2026-10-15T00:00:00Z" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(last, w)
	}
	var value map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &value); e != nil || value["sender"] != "9007199254740993" {
		t.Fatal(value, e)
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
		if last.Keyword != "" || last.SearchSender != 9007199254740993 || last.Conversation != -10 {
			t.Fatal("sender-only query changed scope", last)
		}
	}
	body["sender"] = ""
	delete(body, "from")
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	body["keyword"] = "needle"
	for _, blank := range []bool{false, true} {
		s.archiveLastSearch = time.Time{}
		if blank {
			body["sender"] = ""
		} else {
			delete(body, "sender")
		}
		w = request(s, "POST", path, body, cookie, csrf)
		expectStatus(t, w, 200)
		if last.SearchSender != 0 {
			t.Fatal("sender persisted", last)
		}
		if e := json.Unmarshal(w.Body.Bytes(), &value); e != nil {
			t.Fatal(e)
		}
		var fresh map[string]any
		json.Unmarshal(w.Body.Bytes(), &fresh)
		if _, ok := fresh["sender"]; ok {
			t.Fatal(fresh)
		}
	}
}
