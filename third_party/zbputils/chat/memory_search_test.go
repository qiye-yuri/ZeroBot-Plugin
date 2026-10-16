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

func TestMemorySearchScopeSourceAndModes(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	a.history.Append(10, "event", historyEvent(10, "remember"))
	if e := a.SaveFromSequence(10, "needle 中文🙂 synthetic-agent-key", 1); e != nil {
		t.Fatal(e)
	}
	if e := a.Save(10, "needle disabled"); e != nil {
		t.Fatal(e)
	}
	if e := a.Update(ctx, 10, 2, 1, "disable", ""); e != nil {
		t.Fatal(e)
	}
	p, e := a.SearchEnabled(ctx, 10, "needle", 20)
	if e != nil || !p.Available || p.Mode != "managed" || len(p.Memories) != 1 || p.Scanned != 1 {
		t.Fatal(e, p)
	}
	x := p.Memories[0]
	if x.ID != "1" || x.Revision != "1" || x.SourceSeq != "1" || x.Source != "agent" || !x.Active || strings.Contains(x.Text, "synthetic-agent-key") {
		t.Fatal(x)
	}
	for _, tc := range []struct {
		s   *MemoryStore
		grp int64
	}{{a, -10}, {b, 10}} {
		p, e := tc.s.SearchEnabled(ctx, tc.grp, "needle", 20)
		if e != nil || len(p.Memories) != 0 {
			t.Fatal("scope leaked", e)
		}
	}
	p, e = a.SearchEnabled(ctx, 10, "Needle", 20)
	if e != nil || len(p.Memories) != 0 {
		t.Fatal("matching is not literal case-sensitive")
	}
	if e := os.WriteFile(a.legacyPath(11), []byte("needle legacy"), 0600); e != nil {
		t.Fatal(e)
	}
	p, e = a.SearchEnabled(ctx, 11, "needle", 20)
	if e != nil || p.Available || p.Mode != "legacy" || len(p.Memories) != 0 {
		t.Fatal("legacy exposed", e, p)
	}
	if _, e = a.SearchEnabled(ctx, 10, "", 20); e != ErrMemoryInput {
		t.Fatal(e)
	}
	if _, e = a.SearchEnabled(ctx, 10, "needle", 21); e != ErrMemoryInput {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = a.SearchEnabled(cancelled, 10, "needle", 20); e == nil {
		t.Fatal("cancelled query succeeded")
	}
	if _, e = a.history.db.Exec("DROP TABLE agent_memories"); e != nil {
		t.Fatal(e)
	}
	if _, e = a.SearchEnabled(ctx, 10, "needle", 20); !errors.Is(e, ErrMemory) {
		t.Fatal("read failure hidden", e)
	}
}

func TestMemorySearchScanAndResponseBounds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		text  string
	}{
		{"records", 5001, "unrelated"}, {"bytes", 1200, strings.Repeat("填", 1300)}, {"response", 25, "needle " + strings.Repeat("中🙂", 500)}, {"oversize", 1, strings.Repeat("x", 70000)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := memoryFixture(t)
			tx, e := a.history.db.Begin()
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback()
			for i := 0; i < tc.count; i++ {
				if _, e = tx.Exec(`INSERT INTO agent_memories(conversation,text,source,source_seq,active,revision,created_at,updated_at) VALUES(10,?,'agent',0,1,1,'fixture','fixture')`, tc.text); e != nil {
					t.Fatal(e)
				}
			}
			if e = tx.Commit(); e != nil {
				t.Fatal(e)
			}
			p, e := a.SearchEnabled(context.Background(), 10, "needle", 20)
			if e != nil || !p.Partial {
				t.Fatal(e, p)
			}
			if p.Scanned > 5000 || len(p.Memories) > 20 {
				t.Fatal("scan bound exceeded")
			}
			if tc.name == "records" && p.Scanned != 5000 {
				t.Fatal(p.Scanned)
			}
			if tc.name == "bytes" && p.Scanned >= 1200 {
				t.Fatal("byte limit ignored")
			}
			total := 0
			for _, x := range p.Memories {
				total += len(x.Text)
				if len(x.Text) > 2000 || !utf8.ValidString(x.Text) || !x.Truncated {
					t.Fatal("bad excerpt", x)
				}
			}
			if total > 16384 {
				t.Fatal("response byte limit ignored")
			}
			p, e = a.SearchEnabledFromSource(context.Background(), 10, "", 20, 0, "agent")
			if e != nil || !p.Partial || p.Scanned > 5000 || len(p.Memories) > 20 || p.Source != "agent" {
				t.Fatal("source-only bounds", p, e)
			}
			total = 0
			for _, item := range p.Memories {
				total += len(item.Text)
				if len(item.Text) > 2000 || !utf8.ValidString(item.Text) {
					t.Fatal("source-only excerpt", item)
				}
			}
			if total > 16384 {
				t.Fatal("source-only response byte limit ignored")
			}
		})
	}
}

func TestAgentMemorySearchParametersAndSharedBudget(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	for _, params := range []zero.Params{{}, {"keyword": " "}, {"keyword": "x", "before": "0"}, {"keyword": "x", "before": "01"}, {"keyword": "x", "before": "-1"}, {"keyword": "x", "before": "9223372036854775808"}, {"keyword": "x", "before": json.Number("1")}, {"keyword": "x", "botId": "456"}, {"keyword": "x", "limit": json.Number("0")}, {"keyword": "x", "limit": json.Number("21")}} {
		s := NewHistorySearch(456, -10)
		s.memory = func(context.Context, int64, int64, string, int, int64, string, string, string) (MemorySearchPage, error) {
			t.Fatal("invalid query reached memory")
			return MemorySearchPage{}, nil
		}
		if e := s.Execute(&ag, zero.APIRequest{Action: goba.SearchMemory, Params: params}); e != nil || s.Outcome() != "local_error" {
			t.Fatal(e)
		}
	}
	s := NewHistorySearch(456, -10)
	calls := 0
	s.memory = func(_ context.Context, bot, grp int64, keyword string, limit int, before int64, source, from, until string) (MemorySearchPage, error) {
		calls++
		if bot != 456 || grp != -10 || keyword != "needle" || limit != 1 || before != 9223372036854775807 {
			t.Fatal("scope or parameters changed")
		}
		return MemorySearchPage{}, errors.New("private path / token")
	}
	req := zero.APIRequest{Action: goba.SearchMemory, Params: zero.Params{"keyword": "needle", "limit": json.Number("1"), "before": "9223372036854775807"}}
	if e := s.Execute(&ag, req); e != nil || s.Outcome() != "local_error" {
		t.Fatal(e)
	}
	rows, _, _ := a.history.recent(-10)
	raw := rows[len(rows)-1].payload
	if strings.Contains(raw, "private path") || !strings.Contains(raw, "记忆检索失败") {
		t.Fatal("failure leaked or hidden", raw)
	}
	s.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) { calls++; return ArchivePage{}, nil }
	for _, req := range []zero.APIRequest{{Action: goba.SearchHistory, Params: zero.Params{"keyword": "x"}}, {Action: goba.ReadHistory, Params: zero.Params{"seq": "1"}}} {
		if e := s.Execute(&ag, req); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.Execute(&ag, req); e == nil || calls != 3 {
		t.Fatal("shared quota bypassed", e, calls)
	}
}

func TestAgentRetrievesMemoryOutsideInitialPrompt(t *testing.T) {
	a, _ := memoryFixture(t)
	if e := a.Save(10, "old-memory-Z82"); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 105; i++ {
		if e := a.Save(10, "new unrelated "+strconv.Itoa(i)); e != nil {
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
	calls := 0
	valid := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls++
		if strings.Contains(string(b), "old-memory-Z82") != (calls == 2) {
			valid = false
		}
		if calls == 2 {
			var payload any
			_ = json.Unmarshal(b, &payload)
			foundResponse := false
			var inspect func(any)
			inspect = func(value any) {
				switch v := value.(type) {
				case string:
					if strings.Contains(v, "memories") && strings.Contains(v, "old-memory-Z82") {
						foundResponse = true
					}
				case []any:
					for _, x := range v {
						inspect(x)
					}
				case map[string]any:
					for _, x := range v {
						inspect(x)
					}
				}
			}
			inspect(payload)
			valid = valid && foundResponse
		}
		reply := `{"action":"search_memory","params":{"keyword":"old-memory-Z82","limit":1}}`
		if calls == 2 {
			reply = `{"action":"end_action","params":{}}`
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
		if req.Action == goba.SearchMemory {
			return s.Execute(&ag, req)
		}
		external++
		return nil
	}, func() bool { return true })
	if e != nil || calls != 2 || !valid || external != 0 {
		t.Fatalf("memory retrieval: %v calls=%d valid=%v external=%d", e, calls, valid, external)
	}
	rows, _, e := a.history.recent(10)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, row := range rows {
		if row.kind == "response" {
			var response struct {
				Data MemorySearchPage `json:"data"`
			}
			if json.Unmarshal([]byte(row.payload), &response) == nil && len(response.Data.Memories) == 1 {
				x := response.Data.Memories[0]
				found = x.ID == "1" && x.Revision == "1" && x.Text == "old-memory-Z82"
			}
		}
	}
	if !found {
		t.Fatal("retrieved memory provenance missing")
	}
}
