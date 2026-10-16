package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestMemoryReadExactCurrentVersionAndScope(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	if e := a.history.Append(10, "event", historyEvent(10, "remember")); e != nil {
		t.Fatal(e)
	}
	if e := a.SaveFromSequence(10, "needle synthetic-agent-key", 1); e != nil {
		t.Fatal(e)
	}
	p, e := a.ReadEnabled(ctx, 10, 1, 1)
	if e != nil || !p.Available || len(p.Memories) != 1 || p.Scanned != 1 {
		t.Fatal(e, p)
	}
	x := p.Memories[0]
	if x.ID != "1" || x.Revision != "1" || x.SourceSeq != "1" || x.Source != "agent" || !x.Active || strings.Contains(x.Text, "synthetic-agent-key") {
		t.Fatal(x)
	}
	for _, tc := range []struct {
		s            *MemoryStore
		grp, id, rev int64
	}{{a, -10, 1, 1}, {b, 10, 1, 1}, {a, 10, 999, 1}, {a, 10, 1, 2}} {
		p, e := tc.s.ReadEnabled(ctx, tc.grp, tc.id, tc.rev)
		if e != nil || len(p.Memories) != 0 {
			t.Fatal("scope/version leaked", e)
		}
	}
	if e = a.Update(ctx, 10, 1, 1, "edit", "changed"); e != nil {
		t.Fatal(e)
	}
	p, e = a.ReadEnabled(ctx, 10, 1, 1)
	if e != nil || len(p.Memories) != 0 {
		t.Fatal("old revision returned")
	}
	if e = a.Update(ctx, 10, 1, 2, "disable", ""); e != nil {
		t.Fatal(e)
	}
	p, e = a.ReadEnabled(ctx, 10, 1, 3)
	if e != nil || len(p.Memories) != 0 {
		t.Fatal("disabled memory returned")
	}
	if e = a.Update(ctx, 10, 1, 3, "restore", ""); e != nil {
		t.Fatal(e)
	}
	p, e = a.ReadEnabled(ctx, 10, 1, 4)
	if e != nil || len(p.Memories) != 1 || p.Memories[0].Text != "changed" {
		t.Fatal(e, p)
	}
	// Chat reset does not silently disable independently persisted memory.
	a.history.reset()
	p, e = a.ReadEnabled(ctx, 10, 1, 4)
	if e != nil || len(p.Memories) != 1 {
		t.Fatal("chat reset changed memory")
	}
	if e = os.WriteFile(a.legacyPath(11), []byte("legacy"), 0600); e != nil {
		t.Fatal(e)
	}
	p, e = a.ReadEnabled(ctx, 11, 1, 1)
	if e != nil || p.Available || p.Mode != "legacy" {
		t.Fatal("legacy read", e, p)
	}
}

func TestMemoryReadBoundsCancellationAndErrors(t *testing.T) {
	a, _ := memoryFixture(t)
	ctx := context.Background()
	if e := a.Save(10, "initial"); e != nil {
		t.Fatal(e)
	}
	for _, text := range []string{strings.Repeat("中🙂", 1500), strings.Repeat("x", 70000)} {
		if _, e := a.history.db.Exec("UPDATE agent_memories SET text=? WHERE id=1", text); e != nil {
			t.Fatal(e)
		}
		p, e := a.ReadEnabled(ctx, 10, 1, 1)
		if e != nil || len(p.Memories) != 1 || !p.Partial {
			t.Fatal(e, p)
		}
		x := p.Memories[0]
		if len(x.Text) > 8000 || !utf8.ValidString(x.Text) || !x.Truncated {
			t.Fatal("bound exceeded")
		}
		if len(text) > 65536 && !strings.Contains(x.Text, "未读取正文") {
			t.Fatal("oversize text loaded")
		}
	}
	if _, e := a.ReadEnabled(ctx, 10, 0, 1); e != ErrMemoryInput {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := a.ReadEnabled(cancelled, 10, 1, 1); e == nil {
		t.Fatal("cancel ignored")
	}
	if _, e := a.history.db.Exec("DROP TABLE agent_memories"); e != nil {
		t.Fatal(e)
	}
	if _, e := a.ReadEnabled(ctx, 10, 1, 1); !errors.Is(e, ErrMemory) {
		t.Fatal("read failure hidden", e)
	}
}

func TestAgentMemoryReadValidationAndQuota(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	for _, params := range []zero.Params{{}, {"id": "1"}, {"id": "1", "revision": "0"}, {"id": "01", "revision": "1"}, {"id": json.Number("1"), "revision": "1"}, {"id": "1", "revision": "9223372036854775808"}, {"id": "1", "revision": "1", "active": false}, {"id": "1", "revision": "1", "group_id": "10"}} {
		s := NewHistorySearch(456, -10)
		s.readMemory = func(context.Context, int64, int64, int64, int64) (MemorySearchPage, error) {
			t.Fatal("invalid query reached storage")
			return MemorySearchPage{}, nil
		}
		if e := s.Execute(&ag, zero.APIRequest{Action: goba.ReadMemory, Params: params}); e != nil || s.Outcome() != "local_error" {
			t.Fatal(e)
		}
	}
	s := NewHistorySearch(456, -10)
	calls := 0
	s.readMemory = func(_ context.Context, bot, grp, id, rev int64) (MemorySearchPage, error) {
		calls++
		if bot != 456 || grp != -10 || id != 9007199254740993 || rev != 9007199254740994 {
			t.Fatal("integer or scope lost")
		}
		return MemorySearchPage{}, errors.New("sensitive disk path")
	}
	req := zero.APIRequest{Action: goba.ReadMemory, Params: zero.Params{"id": "9007199254740993", "revision": "9007199254740994"}}
	if e := s.Execute(&ag, req); e != nil {
		t.Fatal(e)
	}
	rows, _, _ := a.history.recent(-10)
	raw := rows[len(rows)-1].payload
	if strings.Contains(raw, "sensitive disk") || !strings.Contains(raw, "记忆读取失败") {
		t.Fatal(raw)
	}
	s.memory = func(context.Context, int64, int64, string, int, int64, string, string, string) (MemorySearchPage, error) {
		calls++
		return MemorySearchPage{}, nil
	}
	s.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) { calls++; return ArchivePage{}, nil }
	for _, req := range []zero.APIRequest{{Action: goba.SearchMemory, Params: zero.Params{"keyword": "x"}}, {Action: goba.SearchHistory, Params: zero.Params{"keyword": "x"}}} {
		if e := s.Execute(&ag, req); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.Execute(&ag, req); e == nil || calls != 3 {
		t.Fatal("shared quota bypassed")
	}
}

func TestAgentReadsLongMemoryAfterSearch(t *testing.T) {
	a, _ := memoryFixture(t)
	if e := a.Save(10, "needle "+strings.Repeat("填", 1000)+" memory-detail-Q91"); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 105; i++ {
		if e := a.Save(10, "unrelated "+strconv.Itoa(i)); e != nil {
			t.Fatal(e)
		}
	}
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	s := NewHistorySearch(456, 10)
	s.memory = func(ctx context.Context, bot, grp int64, keyword string, limit int, before int64, source, from, until string) (MemorySearchPage, error) {
		if bot != 456 {
			t.Fatal(bot)
		}
		return a.SearchEnabledBefore(ctx, grp, keyword, limit, before)
	}
	s.readMemory = func(ctx context.Context, bot, grp, id, rev int64) (MemorySearchPage, error) {
		if bot != 456 {
			t.Fatal(bot)
		}
		return a.ReadEnabled(ctx, grp, id, rev)
	}
	calls := 0
	valid := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls++
		if strings.Contains(string(body), "memory-detail-Q91") != (calls == 3) {
			valid = false
		}
		reply := `{"action":"end_action","params":{}}`
		if calls == 1 {
			reply = `{"action":"search_memory","params":{"keyword":"needle","limit":1}}`
		}
		if calls == 2 {
			reply = `{"action":"read_memory","params":{"id":"1","revision":"1"}}`
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": reply}}}})
	}))
	defer server.Close()
	api := deepinfra.NewAPI(server.URL, "fixture")
	api.SetHTTPClient(server.Client())
	external := 0
	e := RunAgent(func(iter int) ([]zero.APIRequest, bool, error) {
		return NextAgentActions(&ag, false, iter, api, model.NewOpenAI("fixture", "", .7, 1, 512, ""), 10, goba.PermRoleUser)
	}, func(req zero.APIRequest) error {
		if req.Action == goba.SearchMemory || req.Action == goba.ReadMemory {
			return s.Execute(&ag, req)
		}
		external++
		return nil
	}, func() bool { return true })
	if e != nil || calls != 3 || !valid || external != 0 {
		t.Fatalf("memory follow-up: %v calls=%d valid=%v external=%d", e, calls, valid, external)
	}
}
