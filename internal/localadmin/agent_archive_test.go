package localadmin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FloatTech/zbputils/chat"
)

func TestArchiveAuthorizationScopeAndCursor(t *testing.T) {
	s := newTestServer(t)
	calls := 0
	var last chat.ArchiveQuery
	s.cfg.AgentArchive = func(ctx context.Context, bot int64, q chat.ArchiveQuery) (chat.ArchivePage, error) {
		calls++
		last = q
		if bot != 12345678 {
			t.Fatal("wrong bot")
		}
		return chat.ArchivePage{Available: true, Entries: []chat.ArchiveEntry{}, Conversations: []chat.ArchiveConversation{}, Snapshot: 100, Next: 50, More: true}, nil
	}
	base := "/api/agent/history?botId=12345678&type=group&id=10"
	expectStatus(t, request(s, "GET", base, nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	for _, pair := range []struct {
		path string
		code int
	}{{"/api/agent/history?botId=999&type=group&id=10", 404}, {"/api/agent/history?botId=12345678&type=private&id=-10", 400}, {"/api/agent/history?botId=../x&type=group&id=10", 400}, {base + "&archived=bad", 400}} {
		expectStatus(t, request(s, "GET", pair.path, nil, cookie, ""), pair.code)
	}
	if calls != 0 {
		t.Fatal("invalid request reached store")
	}
	w := request(s, "GET", base, nil, cookie, "")
	expectStatus(t, w, 200)
	var page struct {
		NextCursor string `json:"nextCursor"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || page.NextCursor == "" {
		t.Fatal("missing cursor")
	}
	expectStatus(t, request(s, "GET", base+"&cursor="+page.NextCursor, nil, cookie, ""), 200)
	if last.Conversation != 10 || last.Snapshot != 100 || last.Before != 50 {
		t.Fatalf("query: %+v", last)
	}
	for _, path := range []string{strings.Replace(base, "type=group", "type=private", 1), base + "&archived=true", strings.Replace(base, "id=10", "id=11", 1)} {
		expectStatus(t, request(s, "GET", path+"&cursor="+page.NextCursor, nil, cookie, ""), 409)
	}
	expectStatus(t, request(s, "GET", base+"&cursor="+page.NextCursor+"x", nil, cookie, ""), 409)
	clock := s.now()
	s.now = func() time.Time { return clock.Add(16 * time.Minute) }
	expectStatus(t, request(s, "GET", base+"&cursor="+page.NextCursor, nil, cookie, ""), 409)
	s.now = func() time.Time { return clock }
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "keyword": "中文"}
	expectStatus(t, request(s, "POST", "/api/agent/history/search", body, cookie, "wrong"), 403)
	expectStatus(t, request(s, "POST", "/api/agent/history/search", body, cookie, csrf), 200)
	if last.Conversation != -10 || !last.Search || last.Keyword != "中文" {
		t.Fatalf("search: %+v", last)
	}
	expectStatus(t, request(s, "POST", "/api/agent/history/search", body, cookie, csrf), 429)
	s.cfg.AgentArchive = func(context.Context, int64, chat.ArchiveQuery) (chat.ArchivePage, error) {
		return chat.ArchivePage{}, errors.New("private path/secret")
	}
	w = request(s, "GET", base, nil, cookie, "")
	expectStatus(t, w, 503)
	if strings.Contains(w.Body.String(), "secret") {
		t.Fatal("internal error leaked")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cacheable")
	}
}

func TestArchiveSearchBeforeBoundary(t *testing.T) {
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
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "keyword": "needle", "before": "9007199254740993", "archived": true}
	path := "/api/agent/history/search"
	expectStatus(t, request(s, "POST", path, body, nil, ""), 401)
	expectStatus(t, request(s, "POST", path, body, cookie, "wrong"), 403)
	for _, value := range []any{"0", "-1", "01", "1.5", "9223372036854775808", 123} {
		body["before"] = value
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	}
	expectStatus(t, request(s, "GET", "/api/agent/history?botId=12345678&type=private&id=10&before=1", nil, cookie, ""), 400)
	body["before"] = "9007199254740993"
	body["cursor"] = "bad"
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	delete(body, "cursor")
	if calls != 0 {
		t.Fatal("invalid request reached storage")
	}
	w := request(s, "POST", path, body, cookie, csrf)
	expectStatus(t, w, 200)
	if last.Before != 9007199254740993 || last.Conversation != -10 || !last.Search || !last.IncludeResetHistory || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(last)
	}
	s.archiveLastSearch = time.Time{}
	delete(body, "before")
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 200)
	if last.Before != 0 {
		t.Fatal("boundary persisted into unbounded search")
	}
}

func TestArchiveSourceAuthorizationAndParameters(t *testing.T) {
	s := newTestServer(t)
	calls := 0
	var last chat.ArchiveQuery
	s.cfg.AgentArchive = func(_ context.Context, bot int64, q chat.ArchiveQuery) (chat.ArchivePage, error) {
		calls++
		last = q
		if bot != 12345678 {
			t.Fatal("wrong bot")
		}
		return chat.ArchivePage{Available: true, Entries: []chat.ArchiveEntry{{Seq: "9007199254740993", Kind: "event", Text: "synthetic"}}}, nil
	}
	base := "/api/agent/history?botId=12345678&type=private&id=10"
	expectStatus(t, request(s, "GET", base+"&seq=1", nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	for _, bad := range []string{"", "0", "-1", "01", "1.5", "9223372036854775808"} {
		expectStatus(t, request(s, "GET", base+"&seq="+bad, nil, cookie, ""), 400)
	}
	expectStatus(t, request(s, "GET", base+"&seq=1&cursor=invalid", nil, cookie, ""), 400)
	expectStatus(t, request(s, "GET", "/api/agent/conversations?botId=12345678&seq=1", nil, cookie, ""), 400)
	expectStatus(t, request(s, "POST", "/api/agent/history/search", map[string]any{"botId": "12345678", "type": "group", "id": "10", "keyword": "x", "seq": "1"}, cookie, csrf), 400)
	expectStatus(t, request(s, "GET", strings.Replace(base, "12345678", "999", 1)+"&seq=1", nil, cookie, ""), 404)
	if calls != 0 {
		t.Fatal("invalid source request read archive")
	}
	w := request(s, "GET", base+"&seq=9007199254740993&archived=true", nil, cookie, "")
	expectStatus(t, w, 200)
	if last.Sequence != 9007199254740993 || last.Conversation != -10 || !last.IncludeResetHistory || last.Search || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(last, w.Header())
	}
	if !strings.Contains(w.Body.String(), `"seq":"9007199254740993"`) {
		t.Fatal("sequence precision lost")
	}
}
