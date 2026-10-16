package chat

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestArchiveSearchTimeRangePrecisionResetAndScope(t *testing.T) {
	root := t.TempDir()
	h, err := openAgentHistory(root, 456, 900000, historyConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for _, row := range []struct {
		scope       int64
		kind, stamp string
	}{
		{10, "event", "2026-10-15T00:00:00Z"},
		{10, "event", "2026-10-15T00:00:00.000000001Z"},
		{10, "event", "2026-10-15T00:00:00.000000002Z"},
		{10, "event", "2026-10-15T08:00:00.000000001+08:00"},
		{10, "event", "unknown"},
		{-10, "event", "2026-10-15T00:00:00.000000001Z"},
		{10, "reset", "2026-10-15T00:00:00Z"},
		{10, "event", "2026-10-15T00:00:00.000000001Z"},
	} {
		if _, err := h.db.Exec("INSERT INTO history(conversation,kind,payload,created_at) VALUES(?,?,?,?)", row.scope, row.kind, `{"message":"range needle"}`, row.stamp); err != nil {
			t.Fatal(err)
		}
	}
	parse := func(s string) time.Time {
		v, e := time.Parse(time.RFC3339Nano, s)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	lower := parse("2026-10-15T00:00:00.000000001Z")
	upper := parse("2026-10-15T00:00:00.000000002Z")
	for _, keyword := range []string{"needle", "", "  "} {
		for _, tc := range []struct {
			name        string
			from, until time.Time
			before      int64
			archived    bool
			want        []string
			partial     bool
		}{
			{"nanosecond-offset", lower, upper, 0, true, []string{"8", "4", "2"}, true},
			{"reset", lower, upper, 0, false, []string{"8"}, false},
			{"sequence", lower, upper, 4, true, []string{"2"}, false},
			{"lower-only", upper, time.Time{}, 0, true, []string{"3"}, true},
			{"upper-only", time.Time{}, lower, 0, true, []string{"7", "1"}, true},
		} {
			t.Run(tc.name+"/keyword="+keyword, func(t *testing.T) {
				page, err := readArchive(context.Background(), root, 456, ArchiveQuery{Conversation: 10, Search: true, Keyword: keyword, IncludeResetHistory: tc.archived, Before: tc.before, SearchFrom: tc.from, SearchUntil: tc.until}, historyConfig)
				if err != nil {
					t.Fatal(err)
				}
				var got []string
				for _, entry := range page.Entries {
					got = append(got, entry.Seq)
				}
				if !reflect.DeepEqual(got, tc.want) || page.Partial != tc.partial {
					t.Fatalf("got %v partial=%v", got, page.Partial)
				}
			})
		}
	}
	for _, q := range []ArchiveQuery{
		{Conversation: 10, SearchFrom: lower},
		{Conversation: 10, Search: true, Keyword: "x", SearchFrom: upper, SearchUntil: lower},
		{Conversation: 10, Search: true, Keyword: "x", SearchFrom: lower, SearchUntil: lower},
		{Search: true, Keyword: "x", SearchUntil: upper},
		{Conversation: 10, Sequence: 1, SearchFrom: lower},
		{Conversation: 10, Search: true},
	} {
		if _, err := readArchive(context.Background(), root, 456, q, historyConfig); err == nil {
			t.Fatal("invalid range query accepted", q)
		}
	}
}

func TestArchiveTimeFilterKeepsScanBudget(t *testing.T) {
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
		if _, err = tx.Exec("INSERT INTO history(conversation,kind,payload,created_at) VALUES(10,'event',?,?)", fmt.Sprintf(`{"message":"needle %d"}`, i), stamp); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	until, _ := time.Parse(time.RFC3339, "2026-10-16T00:00:00Z")
	for _, keyword := range []string{"needle", ""} {
		q := ArchiveQuery{Conversation: 10, Search: true, Keyword: keyword, SearchUntil: until}
		page, err := readArchive(context.Background(), root, 456, q, historyConfig)
		if err != nil || page.Scanned != 5000 || !page.Partial || len(page.Entries) != 0 || !page.SearchMore {
			t.Fatalf("filter bypassed scan budget: %+v %v", page, err)
		}
		q.Before, _ = strconv.ParseInt(page.NextBefore, 10, 64)
		page, err = readArchive(context.Background(), root, 456, q, historyConfig)
		if err != nil || len(page.Entries) != 1 || page.Entries[0].Seq != "1" || page.SearchMore {
			t.Fatal("empty time window could not continue", page, err)
		}
	}
}
