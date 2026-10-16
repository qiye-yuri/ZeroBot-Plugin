package chat

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestMemoryManualCreationSourceScopeAndReuse(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	item, created, e := a.CreateManual(ctx, 10, "  manual synthetic-agent-key  ")
	if e != nil || !created || item.ID != "1" || item.Revision != "1" || item.Source != "admin" || item.SourceSeq != "0" || !item.Active || strings.Contains(item.Text, "synthetic-agent-key") {
		t.Fatal(e, item)
	}
	loaded, e := a.LoadChecked(10)
	if e != nil || len(loaded) != 1 || !strings.Contains(loaded[0], "manual") {
		t.Fatal("manual memory not loaded", e)
	}
	same, created, e := a.CreateManual(ctx, 10, "manual synthetic-agent-key")
	if e != nil || created || same != item {
		t.Fatal("reuse changed metadata", e, same)
	}
	v, e := a.View(ctx, 10, 0, 1)
	if e != nil || len(v.Items) != 1 {
		t.Fatal("reuse added revision")
	}
	for _, tc := range []struct {
		s   *MemoryStore
		grp int64
	}{{a, -10}, {b, 10}} {
		_, created, e := tc.s.CreateManual(ctx, tc.grp, "manual synthetic-agent-key")
		if e != nil || !created {
			t.Fatal("scope incorrectly reused", e)
		}
	}
	if e = a.Save(20, "agent value"); e != nil {
		t.Fatal(e)
	}
	item, created, e = a.CreateManual(ctx, 20, "agent value")
	if e != nil || created || item.Source != "agent" {
		t.Fatal("agent provenance replaced", e, item)
	}
	id, _ := strconv.ParseInt(item.ID, 10, 64)
	if e = a.Update(ctx, 20, id, 1, "disable", ""); e != nil {
		t.Fatal(e)
	}
	item, created, e = a.CreateManual(ctx, 20, "agent value")
	if e != nil || !created || item.Source != "admin" {
		t.Fatal("disabled row unexpectedly reused", e, item)
	}
}

func TestMemoryManualCreationConcurrencyLegacyAndRollback(t *testing.T) {
	a, _ := memoryFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	var mu sync.Mutex
	createdCount := 0
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, created, e := a.CreateManual(ctx, 10, "same manual")
			if e != nil {
				t.Error(e)
			}
			if created {
				mu.Lock()
				createdCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	v, e := a.View(ctx, 10, 0, 0)
	if e != nil || len(v.Items) != 1 || createdCount != 1 {
		t.Fatal("concurrent duplicates", e, createdCount)
	}
	for _, text := range []string{"", "  ", "line1\nline2", strings.Repeat("中", 1400)} {
		if _, _, e = a.CreateManual(ctx, 10, text); e != ErrMemoryInput {
			t.Fatal("invalid text accepted", e)
		}
	}
	original := []byte("legacy value")
	if e = os.WriteFile(a.legacyPath(11), original, 0600); e != nil {
		t.Fatal(e)
	}
	if _, _, e = a.CreateManual(ctx, 11, "new manual"); e != ErrMemoryConflict {
		t.Fatal("unassigned legacy accepted", e)
	}
	bytes, e := os.ReadFile(a.legacyPath(11))
	if e != nil || string(bytes) != string(original) {
		t.Fatal("legacy modified")
	}
	if _, e = a.history.db.Exec(`CREATE TRIGGER reject_manual_revision BEFORE INSERT ON agent_memory_revisions BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal(e)
	}
	if _, created, e := a.CreateManual(ctx, 12, "rollback manual"); e != ErrMemory || created {
		t.Fatal("failed revision reported success", e)
	}
	v, e = a.View(ctx, 12, 0, 0)
	if e != nil || len(v.Items) != 0 {
		t.Fatal("failed create not rolled back")
	}
}
