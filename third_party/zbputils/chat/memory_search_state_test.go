package chat

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

func TestMemorySearchCurrentStateScopeAndVersions(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	for _, text := range []string{"needle obsolete", "needle disabled", "needle active", "needle legacy"} {
		if err := a.Save(10, text); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Update(ctx, 10, 1, 1, "edit", "current synthetic-agent-key"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2, 4} {
		revision := int64(1)
		if id == 1 {
			revision = 2
		}
		if err := a.Update(ctx, 10, id, revision, "disable", ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.history.db.Exec(`UPDATE agent_memories SET source='legacy' WHERE id=4`); err != nil {
		t.Fatal(err)
	}
	if err := a.Save(-10, "needle private"); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(10, "needle other bot"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ state, source, keyword, ids string }{
		{"active", "", "needle", "3"},
		{"disabled", "", "needle", "4,2"},
		{"all", "", "needle", "4,3,2"},
		{"disabled", "agent", "", "2"},
		{"disabled", "legacy", "", "4"},
		{"disabled", "admin", "", "1"},
	} {
		page, err := a.SearchCurrent(ctx, 10, tc.keyword, 20, 0, tc.source, tc.state)
		ids := []string{}
		for _, item := range page.Memories {
			ids = append(ids, item.ID)
			if (tc.state != "all" && item.Active != (tc.state == "active")) || strings.Contains(item.Text, "synthetic-agent-key") || (item.ID == "1" && item.Revision != "3") {
				t.Fatal("wrong current state, version or redaction", item)
			}
		}
		if err != nil || page.State != tc.state || page.Source != tc.source || strings.Join(ids, ",") != tc.ids {
			t.Fatal(tc, page, err)
		}
	}
	page, err := a.SearchCurrent(ctx, 10, "needle", 1, 0, "", "disabled")
	if err != nil || len(page.Memories) != 1 || page.Memories[0].ID != "4" || !page.SearchMore {
		t.Fatal(page, err)
	}
	before, _ := strconv.ParseInt(page.NextBefore, 10, 64)
	page, err = a.SearchCurrent(ctx, 10, "needle", 1, before, "", "disabled")
	if err != nil || len(page.Memories) != 1 || page.Memories[0].ID != "2" || page.SearchMore {
		t.Fatal(page, err)
	}
	page, err = a.SearchEnabledFromSource(ctx, 10, "needle", 20, 0, "")
	if err != nil || len(page.Memories) != 1 || page.Memories[0].ID != "3" || page.State != "" || strings.Contains(page.Note, "后台状态筛选") {
		t.Fatal("enabled wrapper exposed disabled memory", page, err)
	}
	page, err = a.ReadEnabled(ctx, 10, 2, 2)
	if err != nil || len(page.Memories) != 0 {
		t.Fatal("disabled memory readable by model", page, err)
	}
	for _, scope := range []struct {
		store *MemoryStore
		grp   int64
	}{{a, -10}, {a, 11}, {b, 10}} {
		page, err := scope.store.SearchCurrent(ctx, scope.grp, "needle", 20, 0, "", "disabled")
		if err != nil || len(page.Memories) != 0 {
			t.Fatal("disabled search leaked scope", page, err)
		}
	}
	for _, state := range []string{"", "unknown", "Active"} {
		if _, err := a.SearchCurrent(ctx, 10, "needle", 20, 0, "", state); !errors.Is(err, ErrMemoryInput) {
			t.Fatal("invalid state accepted", state, err)
		}
	}
	if _, err := a.SearchCurrent(ctx, 10, "", 20, 0, "", "disabled"); !errors.Is(err, ErrMemoryInput) {
		t.Fatal("state alone bypassed search conditions", err)
	}
}

func TestMemorySearchDisabledFiltersBeforeScanWindow(t *testing.T) {
	a, _ := memoryFixture(t)
	ctx := context.Background()
	if err := a.Save(10, "needle disabled outside active window"); err != nil {
		t.Fatal(err)
	}
	if err := a.Update(ctx, 10, 1, 1, "disable", ""); err != nil {
		t.Fatal(err)
	}
	_, err := a.history.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<5001) INSERT INTO agent_memories(conversation,text,source,source_seq,active,revision,created_at,updated_at) SELECT 10,'needle active','agent',0,1,1,'fixture','fixture' FROM n`)
	if err != nil {
		t.Fatal(err)
	}
	page, err := a.SearchCurrent(ctx, 10, "needle", 20, 0, "", "disabled")
	if err != nil || page.Scanned != 1 || len(page.Memories) != 1 || page.Memories[0].ID != "1" || page.Partial {
		t.Fatal("state applied after bounded scan", page, err)
	}
	// All-state queries retain the response bound and expose a real continuation.
	page, err = a.SearchCurrent(ctx, 10, "needle", 20, 0, "", "all")
	if err != nil || len(page.Memories) != 20 || !page.SearchMore || page.NextBefore == "" {
		t.Fatal(page, err)
	}
}
