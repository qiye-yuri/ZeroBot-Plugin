package localadmin

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/FloatTech/zbputils/chat"
)

type fakeAgentMemory struct {
	viewRevision [3]int64
	newest       [3]int64
	currentID    int64
	searchSource string
	searchState  string
	searchFrom   string
	searchUntil  string
	calls        int
	grp          int64
	fail         bool
	reverted     [4]int64
	state        string
	keyword      string
	readIDs      [2]int64
	createText   string
	before       int64
}

func (f *fakeAgentMemory) ViewRevision(ctx context.Context, grp, id, revision int64) (chat.MemoryView, error) {
	f.viewRevision = [3]int64{grp, id, revision}
	if f.fail {
		f.calls++
		return chat.MemoryView{}, chat.ErrMemory
	}
	return f.View(ctx, grp, 0, id)
}

func (f *fakeAgentMemory) SearchCurrentInRange(ctx context.Context, grp int64, keyword string, limit int, before int64, source, state, from, until string) (chat.MemorySearchPage, error) {
	f.searchFrom, f.searchUntil = from, until
	p, e := f.SearchCurrent(ctx, grp, keyword, limit, before, source, state)
	start, end, _ := chat.ParseArchiveSearchRange(from, until)
	if !start.IsZero() {
		p.From = start.Format(time.RFC3339Nano)
	}
	if !end.IsZero() {
		p.Until = end.Format(time.RFC3339Nano)
	}
	return p, e
}

func (f *fakeAgentMemory) CreateManual(_ context.Context, grp int64, text string) (chat.MemoryItem, bool, error) {
	f.calls++
	f.grp = grp
	f.createText = text
	if f.fail {
		return chat.MemoryItem{}, false, chat.ErrMemoryConflict
	}
	return chat.MemoryItem{ID: "9007199254740993", Revision: "1", Source: "admin", SourceSeq: "0", Active: true, Text: text}, true, nil
}

func TestMemoryManualCreationAPI(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentMemory{}
	s.cfg.AgentMemory = func(int64) (AgentMemoryStore, error) { return f, nil }
	path := "/api/agent/memories/create"
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "text": "manual value"}
	expectStatus(t, request(s, "POST", path, body, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "POST", path, body, cookie, "bad"), 403)
	body["botId"] = "999"
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 404)
	body["botId"] = "12345678"
	body["memoryId"] = "1"
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	delete(body, "memoryId")
	if f.calls != 0 {
		t.Fatal("unauthorized or mixed create reached storage")
	}
	w := request(s, "POST", path, body, cookie, csrf)
	expectStatus(t, w, 200)
	if f.grp != -10 || f.createText != "manual value" || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), `"created":true`) || !strings.Contains(w.Body.String(), `"id":"9007199254740993"`) {
		t.Fatal(w.Body.String())
	}
	f.fail = true
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 409)
}

func (f *fakeAgentMemory) SearchEnabled(_ context.Context, grp int64, keyword string, limit int) (chat.MemorySearchPage, error) {
	f.calls++
	f.grp = grp
	f.keyword = keyword
	if limit != 20 {
		panic("bad limit")
	}
	if f.fail {
		return chat.MemorySearchPage{}, chat.ErrMemory
	}
	return chat.MemorySearchPage{Available: true, Mode: "managed", Memories: []chat.MemoryMatch{}}, nil
}
func (f *fakeAgentMemory) SearchEnabledBefore(ctx context.Context, grp int64, keyword string, limit int, before int64) (chat.MemorySearchPage, error) {
	f.before = before
	return f.SearchEnabled(ctx, grp, keyword, limit)
}
func (f *fakeAgentMemory) SearchEnabledFromSource(ctx context.Context, grp int64, keyword string, limit int, before int64, source string) (chat.MemorySearchPage, error) {
	f.searchSource = source
	page, err := f.SearchEnabledBefore(ctx, grp, keyword, limit, before)
	page.Source = source
	return page, err
}
func (f *fakeAgentMemory) SearchCurrent(ctx context.Context, grp int64, keyword string, limit int, before int64, source, state string) (chat.MemorySearchPage, error) {
	f.searchState = state
	page, err := f.SearchEnabledFromSource(ctx, grp, keyword, limit, before, source)
	page.State = state
	return page, err
}
func (f *fakeAgentMemory) ReadEnabled(_ context.Context, grp, id, revision int64) (chat.MemorySearchPage, error) {
	f.calls++
	f.grp = grp
	f.readIDs = [2]int64{id, revision}
	if f.fail {
		return chat.MemorySearchPage{}, chat.ErrMemory
	}
	return chat.MemorySearchPage{Available: true, Mode: "managed", Memories: []chat.MemoryMatch{}}, nil
}

func (f *fakeAgentMemory) ReadCurrent(ctx context.Context, grp, id int64) (chat.MemoryView, error) {
	f.calls++
	f.grp, f.currentID = grp, id
	if f.fail {
		return chat.MemoryView{}, chat.ErrMemory
	}
	return chat.MemoryView{Mode: "managed", Items: []chat.MemoryItem{{ID: strconv.FormatInt(id, 10), Revision: "4", Text: "current memory", Active: false}}}, nil
}

func TestMemorySearchAndReadAPI(t *testing.T) {
	s := newTestServer(t)
	store := &fakeAgentMemory{}
	s.cfg.AgentMemory = func(int64) (AgentMemoryStore, error) { return store, nil }
	path := "/api/agent/memories/search"
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "keyword": "needle"}
	expectStatus(t, request(s, "POST", path, body, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "POST", path, body, cookie, "wrong"), 403)
	body["botId"] = "999"
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 404)
	body["botId"] = "12345678"
	for _, word := range []string{"", strings.Repeat("中", 129)} {
		body["keyword"] = word
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	}
	body["keyword"] = "needle"
	body["memoryId"] = "1"
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	delete(body, "memoryId")
	if store.calls != 0 {
		t.Fatal("invalid search reached storage")
	}
	w := request(s, "POST", path, body, cookie, csrf)
	expectStatus(t, w, 200)
	if store.grp != -10 || store.keyword != "needle" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("search scope or caching incorrect")
	}
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 429)
	base := "/api/agent/memories/read?botId=12345678&type=private&id=10&memoryId=9007199254740993&revision=9007199254740994"
	expectStatus(t, request(s, "GET", base, nil, nil, ""), 401)
	for _, tail := range []string{"&after=1", "&state=active", "&before=1", "&before="} {
		expectStatus(t, request(s, "GET", base+tail, nil, cookie, ""), 400)
	}
	for _, bad := range []string{"0", "01", "-1", "9223372036854775808"} {
		expectStatus(t, request(s, "GET", strings.Replace(base, "revision=9007199254740994", "revision="+bad, 1), nil, cookie, ""), 400)
	}
	w = request(s, "GET", base, nil, cookie, "")
	expectStatus(t, w, 200)
	if store.grp != -10 || store.readIDs != [2]int64{9007199254740993, 9007199254740994} || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("read scope/version changed")
	}
	store.fail = true
	expectStatus(t, request(s, "GET", base, nil, cookie, ""), 503)
	s.archiveLastSearch = time.Time{}
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 503)
}

func TestMemorySearchBeforeAPI(t *testing.T) {
	s := newTestServer(t)
	f := &fakeAgentMemory{}
	s.cfg.AgentMemory = func(int64) (AgentMemoryStore, error) { return f, nil }
	path := "/api/agent/memories/search"
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "keyword": "needle", "before": "9223372036854775807"}
	expectStatus(t, request(s, "POST", path, body, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "POST", path, body, cookie, "bad"), 403)
	body["botId"] = "999"
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 404)
	body["botId"] = "12345678"
	for _, bad := range []any{"0", "01", "-1", "+1", "1.0", " 1", "9223372036854775808", json.Number("1"), true} {
		body["before"] = bad
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	}
	if f.calls != 0 {
		t.Fatal("invalid or unauthorized boundary reached storage")
	}
	body["before"] = "9223372036854775807"
	w := request(s, "POST", path, body, cookie, csrf)
	expectStatus(t, w, 200)
	if f.grp != -10 || f.before != 9223372036854775807 || f.keyword != "needle" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("boundary, scope or caching changed")
	}
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 429)
	if f.calls != 1 {
		t.Fatal("rate limit bypassed")
	}
	for _, before := range []string{"", "9007199254740993"} {
		s.archiveLastSearch = time.Time{}
		body["type"], body["before"] = "group", before
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), 200)
		if f.grp != 10 || (before == "" && f.before != 0) || (before != "" && f.before != 9007199254740993) {
			t.Fatal("boundary precision lost")
		}
	}
	s.archiveLastSearch = time.Time{}
	delete(body, "before")
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 200)
	if f.before != 0 {
		t.Fatal("omitted boundary kept previous query")
	}
	// Boundary input belongs exclusively to search, including GET empty values.
	calls := f.calls
	base := "?botId=12345678&type=group&id=10"
	for _, path := range []string{"/api/agent/memories", "/api/agent/memories/read"} {
		for _, tail := range []string{"&before=1", "&before="} {
			expectStatus(t, request(s, "GET", path+base+tail, nil, cookie, ""), 400)
		}
	}
	body["before"] = "1"
	for _, path := range []string{"/api/agent/memories/create", "/api/agent/memories/update", "/api/agent/memories/migration-preview", "/api/agent/memories/migrate"} {
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	}
	if f.calls != calls {
		t.Fatal("mixed operation reached storage")
	}
	f.fail = true
	s.archiveLastSearch = time.Time{}
	w = request(s, "POST", path, body, cookie, csrf)
	expectStatus(t, w, 503)
	if strings.Contains(w.Body.String(), "needle") {
		t.Fatal("failure exposed query")
	}
}

func (f *fakeAgentMemory) ViewFiltered(ctx context.Context, g, a, id int64, state string) (chat.MemoryView, error) {
	f.state = state
	return f.View(ctx, g, a, id)
}
func (f *fakeAgentMemory) ViewVersionsNewest(ctx context.Context, grp, id, before int64) (chat.MemoryView, error) {
	f.newest = [3]int64{grp, id, before}
	if f.fail {
		return chat.MemoryView{}, chat.ErrMemory
	}
	return f.View(ctx, grp, before, id)
}

func TestMemoryStateFilterAPI(t *testing.T) {
	s := newTestServer(t)
	store := &fakeAgentMemory{}
	s.cfg.AgentMemory = func(int64) (AgentMemoryStore, error) { return store, nil }
	cookie, _ := enroll(t, s)
	base := "/api/agent/memories?botId=12345678&type=private&id=10"
	for _, state := range []string{"all", "active", "disabled"} {
		expectStatus(t, request(s, "GET", base+"&state="+state, nil, cookie, ""), 200)
		if store.state != state || store.grp != -10 {
			t.Fatal("wrong filter or scope")
		}
	}
	calls := store.calls
	for _, tail := range []string{"&state=bad", "&memoryId=1&state=active", "&memoryId=1&state=disabled"} {
		expectStatus(t, request(s, "GET", base+tail, nil, cookie, ""), 400)
	}
	if calls != store.calls {
		t.Fatal("invalid filter reached store")
	}
	expectStatus(t, request(s, "GET", base, nil, cookie, ""), 200)
	if store.state != "all" {
		t.Fatal("default changed")
	}
}

func (f *fakeAgentMemory) Revert(_ context.Context, g, id, revision, target int64) error {
	f.calls++
	f.reverted = [4]int64{g, id, revision, target}
	if f.fail {
		return chat.ErrMemoryConflict
	}
	return nil
}

func TestMemoryRevertAPI(t *testing.T) {
	s := newTestServer(t)
	store := &fakeAgentMemory{}
	s.cfg.AgentMemory = func(int64) (AgentMemoryStore, error) { return store, nil }
	body := map[string]any{"botId": "12345678", "type": "private", "id": "10", "memoryId": "9007199254740993", "revision": "9007199254740994", "targetRevision": "9007199254740993", "action": "revert"}
	path := "/api/agent/memories/update"
	expectStatus(t, request(s, "POST", path, body, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "POST", path, body, cookie, "wrong"), 403)
	body["botId"] = "999"
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 404)
	body["botId"] = "12345678"
	for _, target := range []string{"", "0", "-1", "01", "1.5", "9223372036854775808", "9007199254740994", "9007199254740995"} {
		body["targetRevision"] = target
		expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	}
	body["targetRevision"] = "9007199254740993"
	body["text"] = "browser must not supply restored text"
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	body["text"] = ""
	body["action"] = "edit"
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 400)
	if store.calls != 0 {
		t.Fatal("invalid revert reached storage")
	}
	body["action"] = "revert"
	w := request(s, "POST", path, body, cookie, csrf)
	expectStatus(t, w, 200)
	if store.reverted != [4]int64{-10, 9007199254740993, 9007199254740994, 9007199254740993} || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("scope, integer precision or caching incorrect")
	}
	store.fail = true
	expectStatus(t, request(s, "POST", path, body, cookie, csrf), 409)
}

func (f *fakeAgentMemory) View(_ context.Context, g, a, id int64) (chat.MemoryView, error) {
	f.calls++
	f.grp = g
	return chat.MemoryView{Mode: "managed", Items: []chat.MemoryItem{}}, nil
}
func (f *fakeAgentMemory) Update(_ context.Context, g, id, rev int64, action, text string) error {
	f.calls++
	f.grp = g
	if f.fail {
		return chat.ErrMemoryConflict
	}
	return nil
}
func (f *fakeAgentMemory) Preview(_ context.Context, g int64) (chat.MemoryPreview, error) {
	f.calls++
	f.grp = g
	return chat.MemoryPreview{Digest: strings.Repeat("a", 64), Count: 2, Bytes: 30, Mode: "legacy"}, nil
}
func (f *fakeAgentMemory) Migrate(_ context.Context, g int64, digest string) error {
	f.calls++
	f.grp = g
	if digest != strings.Repeat("a", 64) {
		return chat.ErrMemoryConflict
	}
	return nil
}

func TestMemoryAPIRolesVersionsTicketsAndExpiry(t *testing.T) {
	s := newTestServer(t)
	store := &fakeAgentMemory{}
	opens := 0
	s.cfg.AgentMemory = func(bot int64) (AgentMemoryStore, error) {
		opens++
		if bot != 12345678 {
			t.Fatal("unknown bot opened")
		}
		return store, nil
	}
	base := "/api/agent/memories?botId=12345678&type=private&id=10"
	expectStatus(t, request(s, "GET", base, nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "GET", strings.Replace(base, "12345678", "999", 1), nil, cookie, ""), 404)
	if opens != 0 {
		t.Fatal("unauthorized request opened storage")
	}
	expectStatus(t, request(s, "GET", base, nil, cookie, ""), 200)
	if store.grp != -10 {
		t.Fatal("private scope not bound")
	}
	body := map[string]any{"botId": "12345678", "type": "group", "id": "10", "memoryId": "1", "revision": "1", "action": "edit", "text": "合成修订"}
	expectStatus(t, request(s, "POST", "/api/agent/memories/update", body, cookie, "wrong"), 403)
	expectStatus(t, request(s, "POST", "/api/agent/memories/update", body, cookie, csrf), 200)
	store.fail = true
	expectStatus(t, request(s, "POST", "/api/agent/memories/update", body, cookie, csrf), 409)
	scope := map[string]any{"botId": "12345678", "type": "group", "id": "10"}
	w := request(s, "POST", "/api/agent/memories/migration-preview", scope, cookie, csrf)
	expectStatus(t, w, 200)
	var p struct {
		Ticket string `json:"ticket"`
	}
	if json.Unmarshal(w.Body.Bytes(), &p) != nil || p.Ticket == "" {
		t.Fatal("preview missing")
	}
	scope["ticket"] = p.Ticket
	scope["id"] = "11"
	before := store.calls
	expectStatus(t, request(s, "POST", "/api/agent/memories/migrate", scope, cookie, csrf), 409)
	if store.calls != before {
		t.Fatal("retargeted preview executed")
	}
	scope["id"] = "10"
	scope["ticket"] = p.Ticket + "x"
	expectStatus(t, request(s, "POST", "/api/agent/memories/migrate", scope, cookie, csrf), 409)
	scope["ticket"] = p.Ticket
	expectStatus(t, request(s, "POST", "/api/agent/memories/migrate", scope, cookie, csrf), 200)
	clock := s.now()
	s.now = func() time.Time { return clock.Add(6 * time.Minute) }
	expectStatus(t, request(s, "POST", "/api/agent/memories/migrate", scope, cookie, csrf), 409)
}
