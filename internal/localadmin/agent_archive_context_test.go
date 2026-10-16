package localadmin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/FloatTech/zbputils/chat"
)

func TestArchiveWebContextAuthorizationAndScope(t *testing.T) {
	s := newTestServer(t)
	calls := 0
	var last chat.ArchiveQuery
	s.cfg.AgentArchive = func(ctx context.Context, bot int64, q chat.ArchiveQuery) (chat.ArchivePage, error) {
		calls++
		last = q
		if bot != 12345678 {
			t.Fatal(bot)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing timeout")
		}
		return chat.ArchivePage{Available: true, Entries: []chat.ArchiveEntry{{Seq: "9007199254740993", Kind: "event", Text: "synthetic"}}}, nil
	}
	base := "/api/agent/history?botId=12345678&type=private&id=10&seq=9007199254740993&context="
	expectStatus(t, request(s, "GET", base+"1", nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	for _, value := range []string{"", "-1", "4", "01", "1.0", "null", "true", "1&context=2", "1&seq=2", "1&archived=true", "1&cursor=x", "1&before=9", "1&from=x"} {
		expectStatus(t, request(s, "GET", base+value, nil, cookie, ""), 400)
	}
	for _, path := range []string{"/api/agent/history?botId=12345678&type=group&id=10&context=1", "/api/agent/conversations?botId=12345678&seq=1&context=1"} {
		expectStatus(t, request(s, "GET", path, nil, cookie, ""), 400)
	}
	expectStatus(t, request(s, "POST", "/api/agent/history/search", map[string]any{"botId": "12345678", "type": "group", "id": "10", "keyword": "x", "context": 1}, cookie, csrf), 400)
	expectStatus(t, request(s, "GET", strings.Replace(base, "12345678", "999", 1)+"1", nil, cookie, ""), 404)
	if calls != 0 {
		t.Fatal("invalid request reached storage")
	}
	for _, radius := range []string{"0", "1", "2", "3"} {
		w := request(s, "GET", base+radius, nil, cookie, "")
		expectStatus(t, w, 200)
		if last.Context != int(radius[0]-'0') || last.Sequence != 9007199254740993 || last.Conversation != -10 || !last.EventsOnly || last.IncludeResetHistory || last.Search || last.Before != 0 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(last, w.Header())
		}
		var page map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if radius == "0" {
			if _, ok := page["anchorSeq"]; ok {
				t.Fatal(page)
			}
			if _, ok := page["context"]; ok {
				t.Fatal(page)
			}
		} else if page["anchorSeq"] != "9007199254740993" || page["context"] != float64(last.Context) || page["nextCursor"] != "" {
			t.Fatal(page)
		}
	}
	expectStatus(t, request(s, "GET", strings.TrimSuffix(base, "&context=")+"&archived=true", nil, cookie, ""), 200)
	if last.Context != 0 || last.EventsOnly || !last.IncludeResetHistory {
		t.Fatal(last)
	}
}

func TestArchiveWebContextUnavailableAndFailure(t *testing.T) {
	s := newTestServer(t)
	cookie, _ := enroll(t, s)
	base := "/api/agent/history?botId=12345678&type=group&id=10&seq=1&context=1"
	for _, available := range []bool{false, true} {
		s.cfg.AgentArchive = func(context.Context, int64, chat.ArchiveQuery) (chat.ArchivePage, error) {
			return chat.ArchivePage{Available: available, Entries: []chat.ArchiveEntry{}}, nil
		}
		w := request(s, "GET", base, nil, cookie, "")
		expectStatus(t, w, 200)
		var page struct {
			Entries   []chat.ArchiveEntry
			Available bool
			AnchorSeq string
			Context   int
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Entries) != 0 || page.Available != available || page.AnchorSeq != "1" || page.Context != 1 {
			t.Fatal(page, err)
		}
	}
	s.cfg.AgentArchive = func(context.Context, int64, chat.ArchiveQuery) (chat.ArchivePage, error) {
		return chat.ArchivePage{}, errors.New("private secret")
	}
	w := request(s, "GET", base, nil, cookie, "")
	expectStatus(t, w, 503)
	if strings.Contains(w.Body.String(), "secret") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w)
	}
}
