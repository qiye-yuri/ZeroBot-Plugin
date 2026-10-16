package chat

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestArchiveSearchContinuationDoesNotSkipUnreturnedMatch(t *testing.T) {
	root := t.TempDir()
	h, err := openAgentHistory(root, 456, 900000, historyConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for _, text := range []string{"needle old", "gap", "needle middle", "gap", "needle new"} {
		if err = h.Append(10, "event", historyEvent(10, text)); err != nil {
			t.Fatal(err)
		}
	}
	query := ArchiveQuery{Conversation: 10, Search: true, Keyword: "needle", SearchLimit: 1, EventsOnly: true}
	var got []string
	for i := 0; i < 3; i++ {
		page, err := readArchive(context.Background(), root, 456, query, historyConfig)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Entries) != 1 {
			t.Fatal(page)
		}
		got = append(got, page.Entries[0].Seq)
		if i < 2 {
			if !page.SearchMore || !page.Partial || page.NextBefore == "" {
				t.Fatal("missing continuation", page)
			}
			boundary, err := strconv.ParseInt(page.NextBefore, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			want := int64(4 - 2*i)
			if boundary != want {
				t.Fatalf("must include the next unreturned match: %d want %d", boundary, want)
			}
			query.Before = boundary
		} else if page.SearchMore || page.NextBefore != "" {
			t.Fatal("invented continuation at EOF", page)
		}
	}
	if !reflect.DeepEqual(got, []string{"5", "3", "1"}) {
		t.Fatal("skipped/repeated result", got)
	}
	if err = h.reset(); err != nil {
		t.Fatal(err)
	}
	page, err := readArchive(context.Background(), root, 456, query, historyConfig)
	if err != nil || len(page.Entries) != 0 || page.SearchMore {
		t.Fatal("continuation bypassed reset", page, err)
	}
	for _, q := range []ArchiveQuery{{Conversation: 10, SearchLimit: 1}, {Conversation: 10, Search: true, Keyword: "x", SearchLimit: -1}, {Conversation: 10, Search: true, Keyword: "x", SearchLimit: 21}} {
		if _, err = readArchive(context.Background(), root, 456, q, historyConfig); err == nil {
			t.Fatal("invalid search limit accepted", q)
		}
	}
}

func TestArchiveSearchContinuationFromEmptyTimeFilteredScan(t *testing.T) {
	root := t.TempDir()
	h, err := openAgentHistory(root, 456, 900000, historyConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	tx, err := h.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < 5002; i++ {
		stamp := "2026-10-16T00:00:00Z"
		if i == 0 {
			stamp = "2026-10-15T00:00:00Z"
		}
		if _, err = tx.Exec("INSERT INTO history(seq,conversation,kind,payload,created_at) VALUES(?,10,'event',?,?)", 9007199254740993+int64(i), fmt.Sprintf(`{"message":"needle %d"}`, i), stamp); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	until, _ := time.Parse(time.RFC3339, "2026-10-16T00:00:00Z")
	query := ArchiveQuery{Conversation: 10, Search: true, Keyword: "needle", SearchUntil: until, SearchLimit: 1, EventsOnly: true}
	page, err := readArchive(context.Background(), root, 456, query, historyConfig)
	if err != nil || page.Scanned != 5000 || !page.Partial || !page.SearchMore || page.NextBefore != "9007199254740995" || len(page.Entries) != 0 {
		t.Fatal("empty window lacks accurate scanned boundary", page, err)
	}
	query.Before, _ = strconv.ParseInt(page.NextBefore, 10, 64)
	page, err = readArchive(context.Background(), root, 456, query, historyConfig)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Seq != "9007199254740993" || page.SearchMore || page.NextBefore != "" {
		t.Fatal("older window failed", page, err)
	}
	query.Conversation = -10
	page, err = readArchive(context.Background(), root, 456, query, historyConfig)
	if err != nil || len(page.Entries) != 0 || page.SearchMore {
		t.Fatal("cross-scope continuation", page, err)
	}
}

func TestArchiveSearchContinuationByteLimitsAndPartialWithoutMore(t *testing.T) {
	for _, kind := range []string{"scan-bytes", "result-bytes", "oversized", "unknown-time", "clipped"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			h, err := openAgentHistory(root, 456, 900000, historyConfig)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			count := 1
			payload := `{"message":"needle ` + strings.Repeat("x", 3000) + `"}`
			stamp := "2026-10-15T00:00:00Z"
			switch kind {
			case "scan-bytes":
				count = 80
				payload = `{"message":"` + strings.Repeat("x", 60000) + `"}`
			case "result-bytes":
				count = 15
			case "oversized":
				payload = `{"message":"needle ` + strings.Repeat("x", 70000) + `"}`
			case "unknown-time":
				stamp = "unknown"
			}
			tx, err := h.db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			for i := 0; i < count; i++ {
				if _, err = tx.Exec("INSERT INTO history(conversation,kind,payload,created_at) VALUES(10,'event',?,?)", payload, stamp); err != nil {
					t.Fatal(err)
				}
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			q := ArchiveQuery{Conversation: 10, Search: true, Keyword: "needle", EventsOnly: true}
			if kind == "unknown-time" {
				q.SearchFrom, _ = time.Parse(time.RFC3339, "2026-10-14T00:00:00Z")
			}
			page, err := readArchive(context.Background(), root, 456, q, historyConfig)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "scan-bytes" || kind == "result-bytes" {
				if !page.SearchMore || page.NextBefore == "" || !page.Partial {
					t.Fatal("missing byte-bound continuation", page)
				}
				boundary, _ := strconv.ParseInt(page.NextBefore, 10, 64)
				q.Before = boundary
				next, err := readArchive(context.Background(), root, 456, q, historyConfig)
				if err != nil || next.Scanned == 0 {
					t.Fatal("byte-bound continuation did not advance", next, err)
				}
				if kind == "result-bytes" && len(next.Entries) == 0 {
					t.Fatal("unreturned byte-limited match lost")
				}
				if kind == "scan-bytes" && page.Scanned+next.Scanned != 80 {
					t.Fatal("scan byte boundary skipped rows", page.Scanned, next.Scanned)
				}
				if kind == "result-bytes" {
					seen := map[string]bool{}
					for _, entry := range append(page.Entries, next.Entries...) {
						if seen[entry.Seq] {
							t.Fatal("repeated match", entry.Seq)
						}
						seen[entry.Seq] = true
					}
					if len(seen) != 15 {
						t.Fatal("byte-limited result lost", len(seen))
					}
				}
			} else if page.SearchMore || page.NextBefore != "" {
				t.Fatal("partial record fabricated further history", page)
			}
			if (kind == "oversized" || kind == "unknown-time" || kind == "clipped") && !page.Partial {
				t.Fatal("partial coverage missing", page)
			}
		})
	}
}
