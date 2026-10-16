package chat

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestMemorySearchBeforeScopeVersionAndReadOnly(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	if e := a.history.Append(10, "event", historyEvent(10, "remember")); e != nil {
		t.Fatal(e)
	}
	for _, text := range []string{"needle old synthetic-agent-key", "needle recent", "needle disabled"} {
		if e := a.SaveFromSequence(10, text, 1); e != nil {
			t.Fatal(e)
		}
	}
	if e := a.Update(ctx, 10, 3, 1, "disable", ""); e != nil {
		t.Fatal(e)
	}
	if e := a.Update(ctx, 10, 2, 1, "edit", "needle revised"); e != nil {
		t.Fatal(e)
	}
	if e := a.Save(-10, "needle private only"); e != nil {
		t.Fatal(e)
	}
	if e := b.Save(10, "needle other bot only"); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		before  int64
		ids     string
		scanned int
	}{
		{0, "2,1", 2}, {1, "", 0}, {2, "1", 1}, {3, "2,1", 2}, {5, "2,1", 2}, {9223372036854775807, "2,1", 2},
	} {
		p, e := a.SearchEnabledBefore(ctx, 10, "needle", 20, tc.before)
		if e != nil || !p.Available || p.Partial || p.Scanned != tc.scanned {
			t.Fatal(tc, e, p)
		}
		ids := []string{}
		for _, item := range p.Memories {
			ids = append(ids, item.ID)
			if !item.Active || strings.Contains(item.Text, "synthetic-agent-key") || strings.Contains(item.Text, "only") {
				t.Fatal(item)
			}
			if item.ID == "2" && (item.Revision != "2" || item.Text != "needle revised" || item.Source != "admin") {
				t.Fatal(item)
			}
		}
		if strings.Join(ids, ",") != tc.ids {
			t.Fatal(tc, ids)
		}
		if tc.before > 0 && !strings.Contains(p.Note, "小于 "+strconv.FormatInt(tc.before, 10)) {
			t.Fatal(p.Note)
		}
	}
	// The old public method keeps its latest-first contract.
	plain, e := a.SearchEnabled(ctx, 10, "needle", 20)
	if e != nil || len(plain.Memories) != 2 || plain.Memories[0].ID != "2" {
		t.Fatal(e, plain)
	}
	if e = a.history.reset(); e != nil {
		t.Fatal(e)
	}
	p, e := a.SearchEnabledBefore(ctx, 10, "needle", 20, 2)
	if e != nil || len(p.Memories) != 1 || p.Memories[0].ID != "1" {
		t.Fatal("chat reset changed persistent memory search", e, p)
	}
	v, e := a.View(ctx, 10, 0, 1)
	if e != nil || len(v.Items) != 1 || v.Items[0].Revision != "1" {
		t.Fatal("search changed revisions", e, v)
	}
	if e := os.WriteFile(a.legacyPath(11), []byte("needle private legacy"), 0600); e != nil {
		t.Fatal(e)
	}
	p, e = a.SearchEnabledBefore(ctx, 11, "needle", 20, 9223372036854775807)
	if e != nil || p.Available || p.Mode != "legacy" || len(p.Memories) != 0 {
		t.Fatal("legacy exposed", e, p)
	}
	if _, e = a.SearchEnabledBefore(ctx, 10, "needle", 20, -1); !errors.Is(e, ErrMemoryInput) {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = a.SearchEnabledBefore(cancelled, 10, "needle", 20, 2); e == nil {
		t.Fatal("cancelled query succeeded")
	}
}

func TestMemorySearchBeforeKeepsScanAndResponseBudgets(t *testing.T) {
	a, _ := memoryFixture(t)
	ctx := context.Background()
	tx, e := a.history.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	for i := 1; i <= 5002; i++ {
		text := "unrelated"
		if i == 1 || i == 5002 {
			text = "needle " + strconv.Itoa(i)
		}
		if _, e = tx.Exec(`INSERT INTO agent_memories(conversation,text,source,source_seq,active,revision,created_at,updated_at) VALUES(10,?,'agent',0,1,1,'fixture','fixture')`, text); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	p, e := a.SearchEnabledBefore(ctx, 10, "needle", 1, 0)
	if e != nil || len(p.Memories) != 1 || p.Memories[0].ID != "5002" || p.Scanned != 5000 || !p.Partial {
		t.Fatal(e, p)
	}
	p, e = a.SearchEnabledBefore(ctx, 10, "needle", 1, 5002)
	if e != nil || len(p.Memories) != 0 || p.Scanned != 5000 || !p.Partial {
		t.Fatal("boundary bypassed scan limit", e, p)
	}
	p, e = a.SearchEnabledBefore(ctx, 10, "needle", 1, 5001)
	if e != nil || len(p.Memories) != 1 || p.Memories[0].ID != "1" || p.Scanned != 5000 || p.Partial {
		t.Fatal("older window not searched", e, p)
	}
	// Existing byte, excerpt and result caps also apply with a nonzero boundary.
	long := "needle " + strings.Repeat("中🙂", 500)
	if _, e = a.history.db.Exec(`UPDATE agent_memories SET text=? WHERE id BETWEEN 2 AND 26`, long); e != nil {
		t.Fatal(e)
	}
	p, e = a.SearchEnabledBefore(ctx, 10, "needle", 20, 27)
	if e != nil || !p.Partial || len(p.Memories) > 20 {
		t.Fatal(e, p)
	}
	total := 0
	for _, item := range p.Memories {
		total += len(item.Text)
		if len(item.Text) > 2000 || !item.Truncated {
			t.Fatal(item)
		}
	}
	if total > 16384 {
		t.Fatal(total)
	}
	if _, e = a.history.db.Exec(`UPDATE agent_memories SET text=? WHERE id<=1200`, strings.Repeat("填", 1300)); e != nil {
		t.Fatal(e)
	}
	p, e = a.SearchEnabledBefore(ctx, 10, "needle", 20, 1201)
	if e != nil || !p.Partial || p.Scanned >= 1200 {
		t.Fatal("byte budget bypassed", e, p)
	}
	if _, e = a.history.db.Exec(`DROP TABLE agent_memories`); e != nil {
		t.Fatal(e)
	}
	if _, e = a.SearchEnabledBefore(ctx, 10, "needle", 20, 2); !errors.Is(e, ErrMemory) {
		t.Fatal("read failure hidden", e)
	}
}
