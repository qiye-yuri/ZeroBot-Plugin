package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestAgentHistorySearchBoundScopeFailureAndBudget(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	search := NewHistorySearch(456, 10)
	calls := 0
	search.read = func(_ context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
		calls++
		if bot != 456 || q.Conversation != 10 || q.IncludeResetHistory || !q.Search {
			t.Fatal("unbound query")
		}
		return ArchivePage{Entries: []ArchiveEntry{{Seq: "1", Time: "fixture", Text: "旧记录"}, {Seq: "2", Text: "other"}}}, nil
	}
	req := zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"keyword": "测试", "limit": json.Number("1")}}
	if e := search.Execute(&ag, req); e != nil {
		t.Fatal(e)
	}
	if e := search.Execute(&ag, zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"keyword": "测试", "group_id": json.Number("99")}}); e != nil {
		t.Fatal(e)
	}
	if calls != 1 {
		t.Fatal("model scope reached search")
	}
	search.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
		return ArchivePage{}, errors.New("synthetic sensitive path")
	}
	if e := search.Execute(&ag, req); e != nil {
		t.Fatal(e)
	}
	if e := search.Execute(&ag, req); e == nil {
		t.Fatal("fourth lookup accepted")
	}
	rows, _, e := a.history.recent(10)
	if e != nil {
		t.Fatal(e)
	}
	var payload string
	for _, r := range rows {
		payload += r.payload
	}
	if !strings.Contains(payload, "旧记录") || strings.Contains(payload, "synthetic sensitive path") || !strings.Contains(payload, "历史检索失败") {
		t.Fatal("retrieval provenance/error missing")
	}
}

func TestAgentRetrievesHistoryOutsideRecentContext(t *testing.T) {
	a, _ := memoryFixture(t)
	a.history.budget = 8192
	if e := a.history.Append(10, "event", historyEvent(10, "旧代号 secret-code-K19")); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 100; i++ {
		if e := a.history.Append(10, "event", historyEvent(10, fmt.Sprintf("近期无关消息 %d %s", i, strings.Repeat("填充", 50)))); e != nil {
			t.Fatal(e)
		}
	}
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	search := NewHistorySearch(456, 10)
	search.read = func(ctx context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
		return readArchive(ctx, filepath.Join(filepath.Dir(a.legacy.root), "history"), bot, q, historyConfig)
	}
	calls := 0
	valid := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls++
		reply := `{"action":"search_history","params":{"keyword":"旧代号","limit":1}}`
		if calls == 1 && strings.Contains(string(b), "secret-code-K19") {
			valid = false
		}
		if calls == 2 {
			if !strings.Contains(string(b), "secret-code-K19") || !strings.Contains(string(b), "sources") {
				valid = false
			}
			reply = `{"action":"send_group_msg","params":{"group_id":10,"message":"secret-code-K19，来源 #1"}}` + "\n" + `{"action":"end_action","params":{}}`
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
		if req.Action == goba.SearchHistory {
			return search.Execute(&ag, req)
		}
		external++
		if req.Action != "send_group_msg" {
			t.Fatal("local action forwarded")
		}
		return nil
	}, func() bool { return true })
	if e != nil || calls != 2 || external != 1 || !valid {
		t.Fatalf("retrieval round trip failed: %v calls=%d external=%d valid=%v", e, calls, external, valid)
	}
}

func TestAgentReadHistoryValidationScopeAndSharedBudget(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	for _, params := range []zero.Params{
		{}, {"seq": ""}, {"seq": "0"}, {"seq": "-1"}, {"seq": "01"}, {"seq": "1.2"}, {"seq": "9223372036854775808"}, {"seq": json.Number("1")}, {"seq": "1", "group_id": json.Number("99")}, {"seq": "1", "archived": true}, {"seq": "1", "keyword": "x"},
	} {
		s := NewHistorySearch(456, -10)
		s.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
			t.Fatal("invalid parameters reached storage")
			return ArchivePage{}, nil
		}
		if e := s.Execute(&ag, zero.APIRequest{Action: goba.ReadHistory, Params: params}); e != nil {
			t.Fatal(e)
		}
		if s.Outcome() != "local_error" {
			t.Fatal("invalid request succeeded")
		}
	}
	s := NewHistorySearch(456, -10)
	calls := 0
	s.read = func(_ context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
		calls++
		if bot != 456 || q.Conversation != -10 || !q.EventsOnly || q.IncludeResetHistory {
			t.Fatal("unbound scope")
		}
		if calls > 1 && (q.Sequence != 9007199254740993 || q.Search || q.Keyword != "" || q.Before != 0) {
			t.Fatal("sequence lost", q)
		}
		return ArchivePage{Entries: []ArchiveEntry{{Seq: "9007199254740993", Text: "clipped", Truncated: true}}}, nil
	}
	if e := s.Execute(&ag, zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"keyword": "x"}}); e != nil {
		t.Fatal(e)
	}
	req := zero.APIRequest{Action: goba.ReadHistory, Params: zero.Params{"seq": "9007199254740993"}}
	for i := 0; i < 2; i++ {
		if e := s.Execute(&ag, req); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.Execute(&ag, req); e == nil || calls != 3 {
		t.Fatal("shared limit bypassed", e, calls)
	}
	rows, _, e := a.history.recent(-10)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(rows[len(rows)-1].payload, `"partial":true`) {
		t.Fatal("truncation not propagated")
	}
	s = NewHistorySearch(456, -10)
	s.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
		return ArchivePage{}, errors.New("sensitive disk path")
	}
	if e = s.Execute(&ag, req); e != nil {
		t.Fatal(e)
	}
	rows, _, _ = a.history.recent(-10)
	last := rows[len(rows)-1].payload
	if s.Outcome() != "local_error" || strings.Contains(last, "sensitive disk path") || !strings.Contains(last, "读取失败") {
		t.Fatal("read error misreported", last)
	}
}

func TestAgentReadsSourceAfterSearch(t *testing.T) {
	a, _ := memoryFixture(t)
	a.history.budget = 8192
	if e := a.history.Append(10, "event", historyEvent(10, "needle "+strings.Repeat("填充", 600)+" read-detail-V47")); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 30; i++ {
		if e := a.history.Append(10, "event", historyEvent(10, strings.Repeat("近期", 150))); e != nil {
			t.Fatal(e)
		}
	}
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	s := NewHistorySearch(456, 10)
	s.read = func(ctx context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
		return readArchive(ctx, filepath.Join(filepath.Dir(a.legacy.root), "history"), bot, q, historyConfig)
	}
	calls := 0
	valid := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls++
		found := strings.Contains(string(b), "read-detail-V47")
		if found != (calls == 3) {
			valid = false
		}
		reply := `{"action":"end_action","params":{}}`
		if calls == 1 {
			reply = `{"action":"search_history","params":{"keyword":"needle","limit":1}}`
		}
		if calls == 2 {
			reply = `{"action":"read_history","params":{"seq":"1"}}`
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
		if req.Action == goba.SearchHistory || req.Action == goba.ReadHistory {
			return s.Execute(&ag, req)
		}
		external++
		return nil
	}, func() bool { return true })
	if e != nil || calls != 3 || external != 0 || !valid {
		t.Fatalf("source follow-up: err=%v calls=%d external=%d valid=%v", e, calls, external, valid)
	}
	// Exact reads still exclude reset history and tool records.
	var toolSeq int64
	if e = a.history.db.QueryRow("SELECT MAX(seq) FROM history WHERE conversation=10 AND kind='request'").Scan(&toolSeq); e != nil {
		t.Fatal(e)
	}
	toolPage, e := s.read(context.Background(), 456, ArchiveQuery{Conversation: 10, Sequence: toolSeq, EventsOnly: true})
	if e != nil || len(toolPage.Entries) != 0 {
		t.Fatal("tool record read as source", e)
	}
	a.history.reset()
	page, e := s.read(context.Background(), 456, ArchiveQuery{Conversation: 10, Sequence: 1, EventsOnly: true})
	if e != nil || len(page.Entries) != 0 {
		t.Fatal("reset boundary bypassed", e)
	}
}

func TestAgentSearchBeforeValidationAndBinding(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	for _, before := range []any{"", "0", "-1", "01", "1.2", " 2", "9223372036854775808", json.Number("2"), nil} {
		s := NewHistorySearch(456, -10)
		s.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
			t.Fatal("invalid boundary queried")
			return ArchivePage{}, nil
		}
		if e := s.Execute(&ag, zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"keyword": "needle", "before": before}}); e != nil {
			t.Fatal(e)
		}
		if s.Outcome() != "local_error" {
			t.Fatal("invalid boundary accepted", before)
		}
	}
	s := NewHistorySearch(456, -10)
	calls := 0
	s.read = func(_ context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
		calls++
		if bot != 456 || q.Conversation != -10 || q.Before != 9007199254740993 || q.Keyword != "needle" || !q.Search || !q.EventsOnly || q.IncludeResetHistory {
			t.Fatal("boundary or scope lost", q)
		}
		return ArchivePage{Entries: []ArchiveEntry{}}, nil
	}
	req := zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"keyword": "needle", "before": "9007199254740993", "limit": json.Number("1")}}
	for i := 0; i < 3; i++ {
		if e := s.Execute(&ag, req); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.Execute(&ag, req); e == nil || calls != 3 {
		t.Fatal("boundary reset task budget")
	}
}

func TestAgentSearchesOlderMatchBySourceBoundary(t *testing.T) {
	a, _ := memoryFixture(t)
	a.history.budget = 8192
	for _, txt := range []string{"needle older-version-P71", "needle newer-version-P72"} {
		if e := a.history.Append(10, "event", historyEvent(10, txt)); e != nil {
			t.Fatal(e)
		}
	}
	for i := 0; i < 30; i++ {
		if e := a.history.Append(10, "event", historyEvent(10, strings.Repeat("近期", 150))); e != nil {
			t.Fatal(e)
		}
	}
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	s := NewHistorySearch(456, 10)
	s.read = func(ctx context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
		return readArchive(ctx, filepath.Join(filepath.Dir(a.legacy.root), "history"), bot, q, historyConfig)
	}
	calls := 0
	valid := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body := string(b)
		calls++
		if strings.Contains(body, "older-version-P71") != (calls == 3) {
			valid = false
		}
		if calls == 2 && !strings.Contains(body, "newer-version-P72") {
			valid = false
		}
		reply := `{"action":"end_action","params":{}}`
		if calls == 1 {
			reply = `{"action":"search_history","params":{"keyword":"needle","limit":1}}`
		}
		if calls == 2 {
			reply = `{"action":"search_history","params":{"keyword":"needle","limit":1,"before":"2"}}`
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
		if req.Action == goba.SearchHistory {
			return s.Execute(&ag, req)
		}
		external++
		return nil
	}, func() bool { return true })
	if e != nil || calls != 3 || external != 0 || !valid {
		t.Fatalf("older search: %v calls=%d external=%d valid=%v", e, calls, external, valid)
	}
	for _, q := range []ArchiveQuery{
		{Conversation: 10, Search: true, Keyword: "needle", Before: 1, EventsOnly: true},
		{Conversation: -10, Search: true, Keyword: "needle", Before: 2, EventsOnly: true},
	} {
		p, e := s.read(context.Background(), 456, q)
		if e != nil || len(p.Entries) != 0 {
			t.Fatal("exclusive boundary or scope bypassed", e)
		}
	}
	a.history.reset()
	p, e := s.read(context.Background(), 456, ArchiveQuery{Conversation: 10, Search: true, Keyword: "needle", Before: 2, EventsOnly: true})
	if e != nil || len(p.Entries) != 0 {
		t.Fatal("older search bypassed reset", e)
	}
}
