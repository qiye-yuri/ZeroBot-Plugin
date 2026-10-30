package chat

import (
	"context"
	"errors"
	goba "github.com/fumiama/go-onebot-agent"
	"strings"
	"testing"
	"time"
)

func TestRecallRetentionScopeAndRestart(t *testing.T) {
	root := t.TempDir()
	h, e := openAgentHistory(root, 456, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	for _, g := range []int64{10, 20} {
		if e = h.Append(g, "event", historyEvent(g, "RECALLED_SECRET")); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = h.db.Exec(`INSERT INTO agent_summaries VALUES(10,0,1,'OLD_SECRET','now')`); e != nil {
		t.Fatal(e)
	}
	if e = h.Append(10, "event", &goba.Event{PostType: "notice", NoticeType: "group_recall", MessageID: 1, GroupID: 10}); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(h.messageRecallCheck(10, "1", ""), ErrMessageRecalled) {
		t.Fatal("recall missing")
	}
	if h.messageRecallCheck(20, "1", "") != nil || h.messageRecallCheck(10, "1", "other-batch") != nil {
		t.Fatal("scope leak")
	}
	if strings.Contains(historyBody(t, h, 10), "RECALLED_SECRET") || strings.Contains(historyBody(t, h, 10), "OLD_SECRET") {
		t.Fatal("recalled content reached model")
	}
	if !strings.Contains(historyBody(t, h, 20), "RECALLED_SECRET") {
		t.Fatal("other group affected")
	}
	var n int
	if e = h.db.QueryRow(`SELECT COUNT(*) FROM history WHERE payload LIKE '%RECALLED_SECRET%'`).Scan(&n); e != nil || n != 2 {
		t.Fatal("audit lost", n, e)
	}
	if e = h.db.QueryRow(`SELECT COUNT(*) FROM quarantined_summaries WHERE text='OLD_SECRET'`).Scan(&n); e != nil || n != 1 {
		t.Fatal("summary audit lost", n, e)
	}
	h.Close()
	h, e = openAgentHistory(root, 456, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	if !errors.Is(h.messageRecallCheck(10, "1", ""), ErrMessageRecalled) {
		t.Fatal("restart lost recall")
	}
}

func TestRecallArchiveReadAndSearch(t *testing.T) {
	root := t.TempDir()
	h, err := openAgentHistory(root, 456, 900000, historyConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	seq, err := h.appendWithSequence(10, "event", historyEvent(10, "withdrawn-needle"), "")
	if err != nil {
		t.Fatal(err)
	}
	before, err := readArchive(context.Background(), root, 456, ArchiveQuery{Conversation: 10, Sequence: seq}, historyConfig)
	if err != nil || len(before.Entries) != 1 || !strings.Contains(before.Entries[0].Text, "withdrawn-needle") {
		t.Fatal("message unavailable before recall", before, err)
	}
	if err = h.Append(10, "event", &goba.Event{PostType: "notice", NoticeType: "group_recall", MessageID: 1, GroupID: 10}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []ArchiveQuery{
		{Conversation: 10},
		{Conversation: 10, Search: true, Keyword: "withdrawn-needle"},
		{Conversation: 10, Sequence: seq, Context: 1, EventsOnly: true},
	} {
		page, err := readArchive(context.Background(), root, 456, query, historyConfig)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range page.Entries {
			if strings.Contains(entry.Text, "withdrawn-needle") {
				t.Fatal("withdrawn message returned by archive", query)
			}
		}
	}
}

func TestRecallRejectsInFlightSummary(t *testing.T) {
	h := summaryFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	h.scheduleSummary(10, func(SummaryRequest) (string, error) {
		close(started)
		<-release
		return "stale-withdrawn-summary", nil
	})
	defer waitSummary(t, h)
	defer close(release)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("summary not started")
	}
	if err := h.Append(10, "event", &goba.Event{PostType: "notice", NoticeType: "group_recall", MessageID: 1, GroupID: 10}); err != nil {
		t.Fatal(err)
	}
	// Release before checking; deferred cleanup also handles an early failure.
	// The result is checked after completion by a separate cleanup callback.
	t.Cleanup(func() {
		var n int
		if err := h.db.QueryRow(`SELECT COUNT(*) FROM agent_summaries WHERE text='stale-withdrawn-summary'`).Scan(&n); err != nil || n != 0 {
			t.Fatal("late recalled summary committed", n, err)
		}
	})
}

func TestRecallTestBatchIsolation(t *testing.T) {
	h, e := openAgentHistory(t.TempDir(), 456, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	h.appendWithSequence(10, "event", historyEvent(10, "DAILY_VISIBLE"), "")
	h.appendWithSequence(10, "event", &goba.Event{PostType: "notice", NoticeType: "group_recall", MessageID: 1, GroupID: 10}, "synthetic")
	if h.messageRecallCheck(10, "1", "") != nil {
		t.Fatal("test recall affects daily")
	}
	if !strings.Contains(historyBody(t, h, 10), "DAILY_VISIBLE") {
		t.Fatal("daily content excluded")
	}
}
