package chat

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestAgentSenderParametersAndAppliedReceipt(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	for _, value := range []any{nil, true, 123, json.Number("123"), "", "0", "01", "-1", "1.5", "9223372036854775808"} {
		s := NewHistorySearch(456, -10)
		s.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
			t.Fatal("invalid sender reached archive")
			return ArchivePage{}, nil
		}
		if e := s.Execute(&ag, zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"keyword": "needle", "sender": value}}); e != nil || s.Outcome() != "local_error" || s.Usage().Used != 1 || s.Usage().Errors != 1 {
			t.Fatal(value, e, s.Usage())
		}
	}
	for _, tc := range []struct {
		action string
		params zero.Params
	}{{goba.SearchMemory, zero.Params{"keyword": "needle", "sender": "123"}}, {goba.ReadHistory, zero.Params{"seq": "1", "sender": "123"}}, {goba.ReadMemory, zero.Params{"id": "1", "revision": "1", "sender": "123"}}} {
		s := NewHistorySearch(456, -10)
		s.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
			t.Fatal("sender accepted on another action")
			return ArchivePage{}, nil
		}
		s.memory = func(context.Context, int64, int64, string, int, int64, string, string, string) (MemorySearchPage, error) {
			t.Fatal("sender accepted for memory")
			return MemorySearchPage{}, nil
		}
		s.readMemory = func(context.Context, int64, int64, int64, int64) (MemorySearchPage, error) {
			t.Fatal("sender accepted for memory read")
			return MemorySearchPage{}, nil
		}
		if e := s.Execute(&ag, zero.APIRequest{Action: tc.action, Params: tc.params}); e != nil || s.Outcome() != "local_error" {
			t.Fatal(tc, e)
		}
	}
	s := NewHistorySearch(456, -10)
	s.read = func(_ context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
		if bot != 456 || q.Conversation != -10 || q.SearchSender != 9007199254740993 || !q.Search || !q.EventsOnly || q.IncludeResetHistory || q.Before != 9223372036854775807 || q.SearchFrom.Format(time.RFC3339Nano) != "2026-10-15T00:00:00Z" {
			t.Fatal("sender range lost", q)
		}
		return ArchivePage{Entries: []ArchiveEntry{}, Available: true}, nil
	}
	if e := s.Execute(&ag, zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"keyword": "needle", "sender": "9007199254740993", "before": "9223372036854775807", "from": "2026-10-15T08:00:00+08:00"}}); e != nil || s.Outcome() != "ok" || s.Usage().Empty != 1 {
		t.Fatal(e, s.Usage())
	}
	readData := func() map[string]any {
		t.Helper()
		var raw string
		if e := a.history.db.QueryRow(`SELECT payload FROM history WHERE conversation=-10 AND kind='response' ORDER BY seq DESC LIMIT 1`).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var response struct{ Data map[string]any }
		if e := json.Unmarshal([]byte(raw), &response); e != nil {
			t.Fatal(e)
		}
		return response.Data
	}
	if data := readData(); data["sender"] != "9007199254740993" || data["from"] != "2026-10-15T00:00:00Z" {
		t.Fatal(data)
	}
	for _, req := range []zero.APIRequest{{Action: goba.SearchHistory, Params: zero.Params{"keyword": "needle"}}, {Action: goba.ReadHistory, Params: zero.Params{"seq": "1", "context": json.Number("1")}}} {
		s = NewHistorySearch(456, -10)
		s.read = func(_ context.Context, _ int64, q ArchiveQuery) (ArchivePage, error) {
			if q.SearchSender != 0 {
				t.Fatal("sender inherited by independent read", q)
			}
			return ArchivePage{Entries: []ArchiveEntry{}}, nil
		}
		if e := s.Execute(&ag, req); e != nil || s.Outcome() != "ok" {
			t.Fatal(e)
		}
		if _, ok := readData()["sender"]; ok {
			t.Fatal("unapplied sender in receipt")
		}
	}
}

func TestAgentSenderQuotaSurvivesErrorsAndReconstruction(t *testing.T) {
	h, run, ag, s := queryTaskFixture(t)
	s.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
		return ArchivePage{}, errors.New("private path")
	}
	for _, sender := range []any{123, "123"} {
		if e := run.Action(goba.SearchHistory); e != nil {
			t.Fatal(e)
		}
		if e := s.Execute(ag, zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"keyword": "needle", "sender": sender}}); e != nil {
			t.Fatal(e)
		}
		if e := run.Result(s.Outcome()); e != nil {
			t.Fatal(e)
		}
	}
	other, e := NewTaskHistorySearch(456, 10, run)
	if e != nil {
		t.Fatal(e)
	}
	other.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
		return ArchivePage{Entries: []ArchiveEntry{}}, nil
	}
	for i := 0; i < 2; i++ {
		if e := run.Action(goba.SearchHistory); e != nil {
			t.Fatal(e)
		}
		e := other.Execute(ag, zero.APIRequest{Action: goba.SearchHistory, Params: zero.Params{"sender": "123"}})
		if (i == 0 && e != nil) || (i == 1 && e == nil) {
			t.Fatal("quota changed", i, e)
		}
		if e := run.Result(other.Outcome()); e != nil {
			t.Fatal(e)
		}
	}
	if e := run.Finish(nil); e != nil {
		t.Fatal(e)
	}
	v, e := h.tasks.Read(context.Background(), 10, run.info.ID)
	if e != nil || v.Queries.Used != 3 || v.Queries.Errors != 2 || v.Queries.Successful != 1 || v.Queries.Empty != 1 || v.Queries.Denied != 1 || v.Queries.SearchHistory != 3 {
		t.Fatal(v, e)
	}
}

func TestAgentSenderOnlySearch(t *testing.T) {
	a, _ := memoryFixture(t)
	ag := goba.NewAgent(&goba.Config{}, 16, 8, time.Hour, "", a, true, true)
	ag.SetHistoryStore(a.history)
	for _, params := range []zero.Params{
		{"sender": "9007199254740993"},
		{"sender": "9007199254740993", "keyword": ""},
		{"sender": "9007199254740993", "keyword": "  ", "limit": json.Number("1"), "before": "22", "from": "2026-10-15T08:00:00+08:00"},
	} {
		s := NewHistorySearch(456, -10)
		calls := 0
		s.read = func(_ context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
			calls++
			if bot != 456 || q.Conversation != -10 || q.Keyword != "" || q.SearchSender != 9007199254740993 || !q.EventsOnly || !q.Search {
				t.Fatal(q)
			}
			return ArchivePage{Entries: []ArchiveEntry{}, Available: true}, nil
		}
		if err := s.Execute(&ag, zero.APIRequest{Action: goba.SearchHistory, Params: params}); err != nil || calls != 1 || s.Usage().Successful != 1 || s.Usage().Used != 1 {
			t.Fatal(params, err, s.Usage())
		}
	}
	for _, req := range []zero.APIRequest{
		{Action: goba.SearchHistory, Params: zero.Params{}},
		{Action: goba.SearchHistory, Params: zero.Params{"keyword": "  ", "from": "", "until": ""}},
		{Action: goba.SearchHistory, Params: zero.Params{"keyword": nil, "sender": "123"}},
		{Action: goba.SearchHistory, Params: zero.Params{"keyword": 123, "sender": "123"}},
		{Action: goba.SearchMemory, Params: zero.Params{}},
	} {
		s := NewHistorySearch(456, -10)
		s.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
			t.Fatal("invalid query read archive")
			return ArchivePage{}, nil
		}
		s.memory = func(context.Context, int64, int64, string, int, int64, string, string, string) (MemorySearchPage, error) {
			t.Fatal("empty memory keyword accepted")
			return MemorySearchPage{}, nil
		}
		if err := s.Execute(&ag, req); err != nil || s.Usage().Errors != 1 || s.Usage().Used != 1 {
			t.Fatal(req, err, s.Usage())
		}
	}
}
