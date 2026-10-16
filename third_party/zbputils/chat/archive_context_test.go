package chat

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func TestArchiveContextAnchorNeighborsSnapshotAndScope(t *testing.T) {
	root := t.TempDir()
	h, err := openAgentHistory(root, 456, 900000, historyConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for _, row := range []struct {
		scope int64
		kind  string
	}{{10, "event"}, {10, "request"}, {10, "event"}, {-10, "event"}, {10, "event"}, {10, "response"}, {10, "event"}, {10, "end"}, {10, "event"}} {
		if _, err = h.db.Exec("INSERT INTO history(conversation,kind,payload,created_at) VALUES(?,?,?,?)", row.scope, row.kind, `{"message":"synthetic-agent-key <img src=x>"}`, "fixture"); err != nil {
			t.Fatal(err)
		}
	}
	q := ArchiveQuery{Conversation: 10, Sequence: 5, Context: 2, EventsOnly: true}
	read := func(bot int64, q ArchiveQuery) ArchivePage {
		t.Helper()
		p, e := readArchive(context.Background(), root, bot, q, historyConfig)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	p := read(456, q)
	var ids []string
	for _, entry := range p.Entries {
		ids = append(ids, entry.Seq)
		if entry.Kind != "event" || strings.Contains(entry.Text, "synthetic-agent-key") {
			t.Fatal("kind/redaction bypassed", entry)
		}
	}
	if !reflect.DeepEqual(ids, []string{"9", "7", "5", "3", "1"}) || p.SearchMore || p.NextBefore != "" || p.More {
		t.Fatal(p)
	}
	q.Snapshot = 7
	p = read(456, q)
	if len(p.Entries) != 4 || p.Entries[0].Seq != "7" {
		t.Fatal("snapshot leaked", p)
	}
	for _, anchor := range []int64{2, 4, 999} {
		q.Sequence = anchor
		if p = read(456, q); len(p.Entries) != 0 {
			t.Fatal("unverified anchor returned neighbors", anchor, p)
		}
	}
	q.Sequence = 5
	q.Snapshot = 0
	q.Conversation = -10
	if p = read(456, q); len(p.Entries) != 0 {
		t.Fatal("private source leaked", p)
	}
	q.Conversation = 10
	if p = read(789, q); p.Available || len(p.Entries) != 0 {
		t.Fatal("other bot leaked", p)
	}
	if err = h.reset(); err != nil {
		t.Fatal(err)
	}
	if p = read(456, q); len(p.Entries) != 0 {
		t.Fatal("context bypassed reset", p)
	}
	for _, bad := range []ArchiveQuery{{Conversation: 10, Sequence: 5, Context: -1, EventsOnly: true}, {Conversation: 10, Sequence: 5, Context: 4, EventsOnly: true}, {Conversation: 10, Sequence: 5, Context: 1}, {Conversation: 10, Context: 1, EventsOnly: true}, {Conversation: 10, Sequence: 5, Context: 1, EventsOnly: true, IncludeResetHistory: true}, {Conversation: 10, Sequence: 5, Context: 1, EventsOnly: true, Search: true, Keyword: "x"}} {
		if _, e := readArchive(context.Background(), root, 456, bad, historyConfig); e == nil {
			t.Fatal("invalid context query accepted", bad)
		}
	}
}

func TestArchiveContextUTF8AndTotalBudgetKeepsAnchor(t *testing.T) {
	root := t.TempDir()
	h, err := openAgentHistory(root, 456, 900000, historyConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for i := int64(0); i < 7; i++ {
		raw, _ := json.Marshal(map[string]any{"message": strings.Repeat("字🙂", 2000)})
		if _, err = h.db.Exec("INSERT INTO history(seq,conversation,kind,payload,created_at) VALUES(?,10,'event',?,'fixture')", 9007199254740993+i, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	p, err := readArchive(context.Background(), root, 456, ArchiveQuery{Conversation: 10, Sequence: 9007199254740996, Context: 3, EventsOnly: true}, historyConfig)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	anchor := false
	for _, entry := range p.Entries {
		total += len(entry.Text)
		if !utf8.ValidString(entry.Text) {
			t.Fatal("broken UTF-8")
		}
		max := 2000
		if entry.Seq == "9007199254740996" {
			max = 8000
			anchor = true
			if len(entry.Text) < 7993 {
				t.Fatal("anchor lost priority", len(entry.Text))
			}
		}
		if len(entry.Text) > max {
			t.Fatal("per-source bound", entry.Seq, len(entry.Text))
		}
	}
	if !anchor || total > 16384 || len(p.Entries) > 7 || !p.Partial || p.SearchMore {
		t.Fatal(p, total)
	}
	if _, err = h.db.Exec("UPDATE history SET payload=? WHERE seq=?", `{"message":"`+strings.Repeat("x", 70000)+`"}`, 9007199254740996); err != nil {
		t.Fatal(err)
	}
	p, err = readArchive(context.Background(), root, 456, ArchiveQuery{Conversation: 10, Sequence: 9007199254740996, Context: 1, EventsOnly: true}, historyConfig)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Entries) != 3 || !strings.Contains(p.Entries[1].Text, "未读取正文") || !p.Partial {
		t.Fatal("oversized anchor bypassed bound", p)
	}
}

func TestAgentReadHistoryContextParametersQuotaAndReceipt(t *testing.T) {
	h, run, ag, s := queryTaskFixture(t)
	for _, value := range []any{"1", json.Number("1.5"), json.Number("-1"), json.Number("4"), nil, true} {
		search := NewHistorySearch(456, 10)
		search.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
			t.Fatal("invalid radius queried storage")
			return ArchivePage{}, nil
		}
		if err := search.Execute(ag, zero.APIRequest{Action: goba.ReadHistory, Params: zero.Params{"seq": "9007199254740993", "context": value}}); err != nil || search.Outcome() != "local_error" || search.Usage().Errors != 1 {
			t.Fatal(value, err)
		}
	}
	for _, action := range []string{goba.SearchHistory, goba.SearchMemory} {
		search := NewHistorySearch(456, 10)
		search.read = func(context.Context, int64, ArchiveQuery) (ArchivePage, error) {
			t.Fatal("context accepted on search")
			return ArchivePage{}, nil
		}
		search.memory = func(context.Context, int64, int64, string, int, int64, string, string, string) (MemorySearchPage, error) {
			t.Fatal("context accepted on memory search")
			return MemorySearchPage{}, nil
		}
		if err := search.Execute(ag, zero.APIRequest{Action: action, Params: zero.Params{"keyword": "x", "context": json.Number("1")}}); err != nil || search.Outcome() != "local_error" {
			t.Fatal(action, err)
		}
	}
	zeroContext := NewHistorySearch(456, 10)
	zeroContext.read = func(_ context.Context, _ int64, q ArchiveQuery) (ArchivePage, error) {
		if q.Context != 0 || q.Sequence != 1 {
			t.Fatal(q)
		}
		return ArchivePage{Entries: []ArchiveEntry{{Seq: "1", Kind: "event", Text: "exact"}}}, nil
	}
	if err := zeroContext.Execute(ag, zero.APIRequest{Action: goba.ReadHistory, Params: zero.Params{"seq": "1", "context": json.Number("0")}}); err != nil || zeroContext.Outcome() != "ok" {
		t.Fatal(err)
	}
	s.read = func(_ context.Context, bot int64, q ArchiveQuery) (ArchivePage, error) {
		if bot != 456 || q.Conversation != 10 || q.Sequence != 9007199254740993 || q.Context != 2 || !q.EventsOnly || q.IncludeResetHistory || q.Search {
			t.Fatal("unbound context", q)
		}
		entries := []ArchiveEntry{}
		for i := int64(2); i >= -2; i-- {
			entries = append(entries, ArchiveEntry{Seq: strconv.FormatInt(q.Sequence+i, 10), Kind: "event", Text: "context source"})
		}
		return ArchivePage{Entries: entries, Available: true}, nil
	}
	if err := run.Action(goba.ReadHistory); err != nil {
		t.Fatal(err)
	}
	if err := s.Execute(ag, zero.APIRequest{Action: goba.ReadHistory, Params: zero.Params{"seq": "9007199254740993", "context": json.Number("2")}}); err != nil {
		t.Fatal(err)
	}
	if err := run.Result(s.Outcome()); err != nil {
		t.Fatal(err)
	}
	var raw string
	var receipt struct {
		Data struct {
			Sources []ArchiveEntry `json:"sources"`
			Anchor  string         `json:"anchorSeq"`
			Context int            `json:"context"`
		} `json:"data"`
	}
	if err := h.db.QueryRow("SELECT payload FROM history WHERE kind='response' ORDER BY seq DESC LIMIT 1").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(raw), &receipt); err != nil || receipt.Data.Anchor != "9007199254740993" || receipt.Data.Context != 2 || len(receipt.Data.Sources) != 5 {
		t.Fatal(receipt, err)
	}
	other, err := NewTaskHistorySearch(456, 10, run)
	if err != nil || other.Usage().Used != 1 {
		t.Fatal("context radius changed task quota", err)
	}
	if err = run.Finish(nil); err != nil {
		t.Fatal(err)
	}
	v, err := h.tasks.Read(context.Background(), 10, run.info.ID)
	if err != nil || v.Queries.ReadHistory != 1 || v.Queries.Used != 1 || v.Queries.Successful != 1 {
		t.Fatal(v, err)
	}
}
