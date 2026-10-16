package chat

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestArchiveSenderMetadataScopeAndContinuation(t *testing.T) {
	for _, keyword := range []string{"needle", "", "  "} {
		t.Run("keyword="+keyword, func(t *testing.T) {
			root := t.TempDir()
			h, e := openAgentHistory(root, 456, 900000, historyConfig)
			if e != nil {
				t.Fatal(e)
			}
			defer h.Close()
			stamp := "2026-10-15T00:00:00Z"
			rows := []struct {
				scope     int64
				kind, raw string
			}{
				{10, "event", `{"post_type":"message","user_id":9007199254740993,"message":"needle OLD synthetic-old-key"}`},
				{10, "event", `{"post_type":"message","user_id":2,"sender":{"user_id":"9007199254740993"},"message":"needle mentions 9007199254740993"}`},
				{10, "request", `{"post_type":"message","user_id":9007199254740993,"message":"needle TOOL"}`},
				{10, "event", `{"post_type":"notice","user_id":9007199254740993,"message":"needle NOTICE"}`},
				{10, "event", `{"post_type":"message_sent","user_id":9007199254740993,"message":"needle OTHER_EVENT"}`},
				{10, "event", `{"post_type":"message","user_id":"9007199254740993","message":"needle NEW"}`},
				{-10, "event", `{"post_type":"message","user_id":9007199254740993,"message":"needle PRIVATE"}`},
				{10, "event", `{"post_type":"message","sender":{"user_id":"9007199254740993"},"message":"needle UNKNOWN"}`},
			}
			for _, r := range rows {
				if _, e = h.db.Exec(`INSERT INTO history(conversation,kind,payload,created_at) VALUES(?,?,?,?)`, r.scope, r.kind, r.raw, stamp); e != nil {
					t.Fatal(e)
				}
			}
			q := ArchiveQuery{Conversation: 10, Search: true, Keyword: keyword, SearchSender: 9007199254740993, SearchLimit: 1}
			p, e := readArchive(context.Background(), root, 456, q, historyConfig)
			if e != nil || len(p.Entries) != 1 || p.Entries[0].Seq != "6" || !p.SearchMore || !p.Partial || p.NextBefore != "2" {
				t.Fatal(p, e)
			}
			q.Before, _ = strconv.ParseInt(p.NextBefore, 10, 64)
			p, e = readArchive(context.Background(), root, 456, q, historyConfig)
			if e != nil || len(p.Entries) != 1 || p.Entries[0].Seq != "1" || p.SearchMore || strings.Contains(p.Entries[0].Text, "synthetic-old-key") {
				t.Fatal("continuation/redaction", p, e)
			}
			q.Before = 0
			q.SearchLimit = 0
			q.Snapshot = 4
			p, e = readArchive(context.Background(), root, 456, q, historyConfig)
			if e != nil || len(p.Entries) != 1 || p.Entries[0].Seq != "1" || p.Partial {
				t.Fatal("snapshot/notice/tools", p, e)
			}
			q.Snapshot = 0
			q.Conversation = -10
			p, e = readArchive(context.Background(), root, 456, q, historyConfig)
			if e != nil || len(p.Entries) != 1 || p.Entries[0].Seq != "7" {
				t.Fatal("private", p, e)
			}
			q.Conversation = 10
			q.SearchFrom, _ = time.Parse(time.RFC3339Nano, stamp)
			q.SearchUntil = q.SearchFrom.Add(time.Nanosecond)
			p, e = readArchive(context.Background(), root, 456, q, historyConfig)
			if e != nil || len(p.Entries) != 2 {
				t.Fatal("range combination", p, e)
			}
			q.SearchUntil = q.SearchFrom
			q.SearchFrom = time.Time{}
			p, e = readArchive(context.Background(), root, 456, q, historyConfig)
			if e != nil || len(p.Entries) != 0 {
				t.Fatal("exclusive time end", p, e)
			}
			q.SearchUntil = time.Time{}
			if e = h.reset(); e != nil {
				t.Fatal(e)
			}
			p, e = readArchive(context.Background(), root, 456, q, historyConfig)
			if e != nil || len(p.Entries) != 0 {
				t.Fatal("reset", p, e)
			}
			q.IncludeResetHistory = true
			p, e = readArchive(context.Background(), root, 456, q, historyConfig)
			if e != nil || len(p.Entries) != 2 {
				t.Fatal("explicit archive", p, e)
			}
			other, e := openAgentHistory(root, 789, 900000, historyConfig)
			if e != nil {
				t.Fatal(e)
			}
			defer other.Close()
			p, e = readArchive(context.Background(), root, 789, q, historyConfig)
			if e != nil || len(p.Entries) != 0 {
				t.Fatal("bot scope", p, e)
			}
		})
	}
}

func TestArchiveSenderUnknownMetadataAndInvalidModes(t *testing.T) {
	root := t.TempDir()
	h, e := openAgentHistory(root, 456, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	for _, raw := range []string{`{}`, `[]`, `{"post_type":"message"}`, `{"post_type":"message","user_id":0}`, `{"post_type":"message","user_id":true}`, `{"post_type":"message","user_id":1.5}`, `{"post_type":"message","user_id":"01"}`, `{"post_type":"message","user_id":1e2}`, `{"post_type":"message","user_id":"9223372036854775808"}`} {
		if _, e = h.db.Exec(`INSERT INTO history(conversation,kind,payload,created_at) VALUES(10,'event',?,'2026-10-15T00:00:00Z')`, raw); e != nil {
			t.Fatal(e)
		}
	}
	q := ArchiveQuery{Conversation: 10, Search: true, Keyword: "needle", SearchSender: 123}
	p, e := readArchive(context.Background(), root, 456, q, historyConfig)
	if e != nil || len(p.Entries) != 0 || !p.Partial || p.SearchMore || p.NextBefore != "" || p.Scanned != 9 {
		t.Fatal(p, e)
	}
	for _, invalid := range []ArchiveQuery{{SearchSender: -1}, {SearchSender: 123}, {Conversation: 10, Sequence: 1, SearchSender: 123}, {Search: true, Keyword: "needle", SearchSender: 123}} {
		if _, e = readArchive(context.Background(), root, 456, invalid, historyConfig); e == nil {
			t.Fatal("invalid sender mode", invalid)
		}
	}
	if _, e = h.db.Exec(`INSERT INTO history(conversation,kind,payload,created_at) VALUES(10,'event',?,'2026-10-15T00:00:00Z')`, strings.Repeat("x", 70000)); e != nil {
		t.Fatal(e)
	}
	p, e = readArchive(context.Background(), root, 456, q, historyConfig)
	if e != nil || !p.Partial || len(p.Entries) != 0 {
		t.Fatal("oversized sender source", p, e)
	}
}

func TestArchiveSenderNonmatchingRowsConsumeScanBudget(t *testing.T) {
	root := t.TempDir()
	h, e := openAgentHistory(root, 456, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer h.Close()
	tx, e := h.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	for i := 0; i < 5002; i++ {
		sender := 2
		if i == 0 {
			sender = 123
		}
		raw := fmt.Sprintf(`{"post_type":"message","user_id":%d,"message":"needle"}`, sender)
		if _, e = tx.Exec(`INSERT INTO history(conversation,kind,payload,created_at) VALUES(10,'event',?,'2026-10-15T00:00:00Z')`, raw); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	q := ArchiveQuery{Conversation: 10, Search: true, Keyword: "needle", SearchSender: 123}
	p, e := readArchive(context.Background(), root, 456, q, historyConfig)
	if e != nil || p.Scanned != 5000 || len(p.Entries) != 0 || !p.SearchMore || p.NextBefore != "3" {
		t.Fatal("sender bypassed scan budget", p, e)
	}
	q.Before = 3
	p, e = readArchive(context.Background(), root, 456, q, historyConfig)
	if e != nil || len(p.Entries) != 1 || p.Entries[0].Seq != "1" || p.SearchMore {
		t.Fatal("sender empty-window continuation", p, e)
	}
	// A mismatching author still consumes the original 4 MiB raw-byte budget.
	if _, e = h.db.Exec(`DELETE FROM history`); e != nil {
		t.Fatal(e)
	}
	tx, e = h.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	for i := 0; i < 90; i++ {
		raw := `{"post_type":"message","user_id":2,"message":"` + strings.Repeat("x", 60000) + `"}`
		if _, e = tx.Exec(`INSERT INTO history(conversation,kind,payload,created_at) VALUES(10,'event',?,'2026-10-15T00:00:00Z')`, raw); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	q.Before = 0
	p, e = readArchive(context.Background(), root, 456, q, historyConfig)
	if e != nil || p.Scanned >= 90 || len(p.Entries) != 0 || !p.SearchMore || !p.Partial {
		t.Fatal("sender bypassed byte budget", p, e)
	}
}
