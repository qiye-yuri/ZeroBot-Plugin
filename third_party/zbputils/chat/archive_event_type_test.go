package chat

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestArchiveEventTypeScopeMetadataAndContinuation(t *testing.T) {
	root := t.TempDir()
	h, err := openAgentHistory(root, 456, 900000, historyConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for _, row := range []struct {
		grp       int64
		kind, raw string
	}{
		{10, "event", `{"post_type":"message","user_id":123,"message":"needle OLD synthetic-old-key"}`},
		{10, "request", `{"post_type":"request","message":"needle TOOL"}`},
		{10, "event", `{"post_type":"notice","message":"needle NOTICE"}`},
		{10, "event", `{"post_type":"request","message":"needle REQUEST"}`},
		{10, "event", `{"post_type":"message","user_id":456,"message":"needle NEW mentions notice"}`},
		{10, "event", `{"message":{"post_type":"message"}}`},
		{-10, "event", `{"post_type":"message","message":"needle PRIVATE"}`},
		{10, "event", `{"post_type":123,"message":"needle UNKNOWN"}`},
		{10, "event", `{"post_type":"message_sent","message":"needle OTHER"}`},
	} {
		if _, err := h.db.Exec(`INSERT INTO history(conversation,kind,payload,created_at) VALUES(?,?,?,'2026-10-15T00:00:00Z')`, row.grp, row.kind, row.raw); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	q := ArchiveQuery{Conversation: 10, Search: true, SearchEventType: "message", SearchLimit: 1}
	page, err := readArchive(ctx, root, 456, q, historyConfig)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Seq != "5" || !page.Partial || !page.SearchMore || page.NextBefore != "2" {
		t.Fatal(page, err)
	}
	q.Before, _ = strconv.ParseInt(page.NextBefore, 10, 64)
	page, err = readArchive(ctx, root, 456, q, historyConfig)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Seq != "1" || page.SearchMore || strings.Contains(page.Entries[0].Text, "synthetic-old-key") {
		t.Fatal(page, err)
	}
	q.Before, q.SearchLimit = 0, 20
	for _, tc := range []struct{ eventType, keyword, ids string }{
		{"notice", "needle", "3"}, {"request", "", "4"}, {"message", "notice", "5"},
	} {
		q.SearchEventType, q.Keyword = tc.eventType, tc.keyword
		page, err := readArchive(ctx, root, 456, q, historyConfig)
		if err != nil || len(page.Entries) != 1 || page.Entries[0].Seq != tc.ids {
			t.Fatal(tc, page, err)
		}
	}
	q.SearchEventType, q.Keyword, q.SearchSender = "notice", "", 123
	page, err = readArchive(ctx, root, 456, q, historyConfig)
	if err != nil || len(page.Entries) != 0 {
		t.Fatal("sender must still select message authors only", page, err)
	}
	q.SearchEventType, q.SearchSender, q.Snapshot = "message", 0, 4
	page, err = readArchive(ctx, root, 456, q, historyConfig)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Seq != "1" {
		t.Fatal("snapshot changed", page, err)
	}
	q.Snapshot, q.Conversation = 0, -10
	page, err = readArchive(ctx, root, 456, q, historyConfig)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Seq != "7" {
		t.Fatal("private scope changed", page, err)
	}
	q.Conversation = 10
	if err = h.reset(); err != nil {
		t.Fatal(err)
	}
	page, err = readArchive(ctx, root, 456, q, historyConfig)
	if err != nil || len(page.Entries) != 0 {
		t.Fatal("reset ignored", page, err)
	}
	q.IncludeResetHistory = true
	page, err = readArchive(ctx, root, 456, q, historyConfig)
	if err != nil || len(page.Entries) != 2 {
		t.Fatal(page, err)
	}
	for _, invalid := range []ArchiveQuery{{Conversation: 10, SearchEventType: "message"}, {Search: true, SearchEventType: "notice"}, {Conversation: 10, Search: true, SearchEventType: "response"}} {
		if _, err := readArchive(ctx, root, 456, invalid, historyConfig); err == nil {
			t.Fatal("invalid mode accepted", invalid)
		}
	}
}

func TestArchiveEventTypeDoesNotBypassScanBudget(t *testing.T) {
	root := t.TempDir()
	h, err := openAgentHistory(root, 456, 900000, historyConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if _, err = h.db.Exec(`INSERT INTO history(conversation,kind,payload,created_at) VALUES(10,'event','{"post_type":"message","message":"old"}','2026-10-15T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err = h.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<5001) INSERT INTO history(conversation,kind,payload,created_at) SELECT 10,'event','{"post_type":"notice"}','2026-10-15T00:00:00Z' FROM n`); err != nil {
		t.Fatal(err)
	}
	q := ArchiveQuery{Conversation: 10, Search: true, SearchEventType: "message"}
	page, err := readArchive(context.Background(), root, 456, q, historyConfig)
	if err != nil || page.Scanned != 5000 || len(page.Entries) != 0 || !page.SearchMore {
		t.Fatal("type filter bypassed bounded scan", page, err)
	}
	q.Before, _ = strconv.ParseInt(page.NextBefore, 10, 64)
	page, err = readArchive(context.Background(), root, 456, q, historyConfig)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Seq != "1" || page.SearchMore {
		t.Fatal(page, err)
	}
}
