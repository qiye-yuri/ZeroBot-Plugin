package chat

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func memoryFixture(t *testing.T) (*MemoryStore, *MemoryStore) {
	t.Helper()
	root := t.TempDir()
	legacy := &memstorage{root: filepath.Join(root, "legacy")}
	if e := os.Mkdir(legacy.root, 0700); e != nil {
		t.Fatal(e)
	}
	makeStore := func(bot int64) *MemoryStore {
		h, e := openAgentHistory(filepath.Join(root, "history"), bot, 900000, historyConfig)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { h.Close() })
		return &MemoryStore{h, legacy, bot}
	}
	return makeStore(456), makeStore(789)
}

func TestMemoryFilteredPagesAndHistory(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	for i := 1; i <= 105; i++ {
		if e := a.Save(10, "memory "+strconv.Itoa(i)); e != nil {
			t.Fatal(e)
		}
		if i%2 == 1 {
			if e := a.Update(ctx, 10, int64(i), 1, "disable", ""); e != nil {
				t.Fatal(e)
			}
		}
	}
	for _, tc := range []struct {
		state string
		want  int
	}{{"all", 105}, {"active", 52}, {"disabled", 53}} {
		seen := map[string]bool{}
		after := int64(0)
		for {
			v, e := a.ViewFiltered(ctx, 10, after, 0, tc.state)
			if e != nil {
				t.Fatal(e)
			}
			if len(v.Items) > 50 {
				t.Fatal("page limit exceeded")
			}
			for _, item := range v.Items {
				if seen[item.ID] || (tc.state == "active" && !item.Active) || (tc.state == "disabled" && item.Active) {
					t.Fatal("filter or pagination failed", item)
				}
				seen[item.ID] = true
			}
			if !v.More {
				break
			}
			after, e = strconv.ParseInt(v.Next, 10, 64)
			if e != nil {
				t.Fatal(e)
			}
		}
		if len(seen) != tc.want {
			t.Fatal(tc.state, len(seen))
		}
	}
	for _, scope := range []struct {
		s   *MemoryStore
		grp int64
	}{{a, -10}, {b, 10}} {
		v, e := scope.s.ViewFiltered(ctx, scope.grp, 0, 0, "disabled")
		if e != nil || len(v.Items) != 0 {
			t.Fatal("scope leaked")
		}
	}
	if _, e := a.ViewFiltered(ctx, 10, 0, 0, "bad"); e != ErrMemoryInput {
		t.Fatal(e)
	}
	if _, e := a.ViewFiltered(ctx, 10, 0, 1, "disabled"); e != ErrMemoryInput {
		t.Fatal("filtered revision history", e)
	}
	v, e := a.View(ctx, 10, 0, 1)
	if e != nil || len(v.Items) != 2 || !v.Items[0].Active || v.Items[1].Active {
		t.Fatal("history changed", e, v)
	}
}

func TestMemoryDuplicateSavePreservesProvenanceAndVersions(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	if e := a.history.Append(10, "event", historyEvent(10, "remember")); e != nil {
		t.Fatal(e)
	}
	if e := a.SaveFromSequence(10, "  exact memory synthetic-agent-key  ", 1); e != nil {
		t.Fatal(e)
	}
	before, e := a.View(ctx, 10, 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	// Both raw and already-redacted saves converge to persisted text.
	for _, text := range []string{"exact memory synthetic-agent-key", before.Items[0].Text} {
		if e = a.SaveFrom(10, text, "999"); e != nil {
			t.Fatal(e)
		}
	}
	after, e := a.View(ctx, 10, 0, 0)
	if e != nil || len(after.Items) != 1 || after.Items[0] != before.Items[0] {
		t.Fatal("duplicate changed metadata", e, after)
	}
	versions, e := a.View(ctx, 10, 0, 1)
	if e != nil || len(versions.Items) != 1 {
		t.Fatal("duplicate added revision", e)
	}
	if e = a.Update(ctx, 10, 1, 1, "edit", "admin text"); e != nil {
		t.Fatal(e)
	}
	if e = a.SaveFrom(10, "admin text", "999"); e != nil {
		t.Fatal(e)
	}
	after, e = a.View(ctx, 10, 0, 0)
	if e != nil || len(after.Items) != 1 || after.Items[0].Source != "admin" || after.Items[0].Revision != "2" {
		t.Fatal("admin provenance changed")
	}
	if e = a.Save(-10, "admin text"); e != nil {
		t.Fatal(e)
	}
	if e = b.Save(10, "admin text"); e != nil {
		t.Fatal(e)
	}
	for _, scope := range []struct {
		s   *MemoryStore
		grp int64
	}{{a, -10}, {b, 10}} {
		v, e := scope.s.View(ctx, scope.grp, 0, 0)
		if e != nil || len(v.Items) != 1 {
			t.Fatal("dedup crossed scope", e)
		}
	}
	if e = a.Update(ctx, 10, 1, 2, "disable", ""); e != nil {
		t.Fatal(e)
	}
	if e = a.Save(10, "admin text"); e != nil {
		t.Fatal(e)
	}
	after, e = a.View(ctx, 10, 0, 0)
	if e != nil || len(after.Items) != 2 || after.Items[0].Active || !after.Items[1].Active {
		t.Fatal("disabled row was overwritten")
	}
	if e = a.Save(10, "Admin text"); e != nil {
		t.Fatal(e)
	}
	after, e = a.View(ctx, 10, 0, 0)
	if e != nil || len(after.Items) != 3 {
		t.Fatal("case-sensitive text incorrectly merged")
	}
}

func TestMemoryDuplicateConcurrentSavesAndReadFailure(t *testing.T) {
	a, _ := memoryFixture(t)
	errs := make(chan error, 24)
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- a.Save(10, "same concurrent memory") }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	v, e := a.View(context.Background(), 10, 0, 0)
	if e != nil || len(v.Items) != 1 || v.Items[0].Revision != "1" {
		t.Fatal("concurrent duplicates", e, v)
	}
	loaded, e := a.LoadChecked(10)
	if e != nil || len(loaded) != 1 {
		t.Fatal("duplicates consumed prompt budget")
	}
	// Repeating a save must still inspect storage and surface a read failure.
	if _, e = a.history.db.Exec("DROP TABLE agent_memories"); e != nil {
		t.Fatal(e)
	}
	if e = a.Save(10, "same concurrent memory"); e != ErrMemory {
		t.Fatal("lookup failure hidden", e)
	}
}

func TestMemoryRevertPreservesHistoryStateAndScope(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	if e := a.history.Append(10, "event", historyEvent(10, "original")); e != nil {
		t.Fatal(e)
	}
	if e := a.SaveFromSequence(10, "original", 1); e != nil {
		t.Fatal(e)
	}
	if e := a.Update(ctx, 10, 1, 1, "edit", "new text"); e != nil {
		t.Fatal(e)
	}
	if e := a.Update(ctx, 10, 1, 2, "disable", ""); e != nil {
		t.Fatal(e)
	}
	for _, target := range []int64{-1, 0, 3, 4} {
		if e := a.Revert(ctx, 10, 1, 3, target); e != ErrMemoryInput {
			t.Fatalf("invalid target %d: %v", target, e)
		}
	}
	if e := a.Revert(ctx, -10, 1, 3, 1); e != ErrMemoryConflict {
		t.Fatalf("scope: %v", e)
	}
	if e := b.Revert(ctx, 10, 1, 3, 1); e != ErrMemoryConflict {
		t.Fatalf("robot: %v", e)
	}
	if e := a.Revert(ctx, 10, 1, 2, 1); e != ErrMemoryConflict {
		t.Fatalf("stale: %v", e)
	}
	// A previously stored value must be redacted with the current configuration too.
	if _, e := a.history.db.Exec("UPDATE agent_memory_revisions SET text='original synthetic-agent-key' WHERE memory_id=1 AND revision=1"); e != nil {
		t.Fatal(e)
	}
	if e := a.Revert(ctx, 10, 1, 3, 1); e != nil {
		t.Fatal(e)
	}
	v, e := a.View(ctx, 10, 0, 0)
	if e != nil || len(v.Items) != 1 {
		t.Fatalf("view: %v %+v", e, v)
	}
	x := v.Items[0]
	if x.Revision != "4" || x.Active || x.Source != "admin" || x.SourceSeq != "1" || !strings.Contains(x.Text, "original") || strings.Contains(x.Text, "synthetic-agent-key") {
		t.Fatalf("restored: %+v", x)
	}
	loaded, e := a.LoadChecked(10)
	if e != nil || len(loaded) != 0 {
		t.Fatal("revert unexpectedly enabled memory")
	}
	versions, e := a.View(ctx, 10, 0, 1)
	if e != nil || len(versions.Items) != 4 || versions.Items[1].Text != "new text" {
		t.Fatal("history changed")
	}
	if e := a.Revert(ctx, 10, 1, 3, 1); e != ErrMemoryConflict {
		t.Fatal("duplicate wrote again")
	}
	// A failed revision insert must roll back the changed current row.
	if _, e := a.history.db.Exec(`CREATE TRIGGER reject_revision BEFORE INSERT ON agent_memory_revisions BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); e != nil {
		t.Fatal(e)
	}
	if e := a.Revert(ctx, 10, 1, 4, 2); e != ErrMemory {
		t.Fatalf("write failure: %v", e)
	}
	v, e = a.View(ctx, 10, 0, 0)
	if e != nil || v.Items[0] != x {
		t.Fatal("failed revert was not rolled back")
	}
}

func TestMemoryIsolationVersionAndSource(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	if e := a.history.Append(10, "event", historyEvent(10, "记住测试")); e != nil {
		t.Fatal(e)
	}
	if e := a.SaveFromSequence(10, "记住 synthetic-agent-key", 1); e != nil {
		t.Fatal(e)
	}
	v, e := a.View(ctx, 10, 0, 0)
	if e != nil || len(v.Items) != 1 || v.Items[0].SourceSeq == "0" || strings.Contains(v.Items[0].Text, "synthetic-agent-key") {
		t.Fatalf("view %v %+v", e, v)
	}
	for _, s := range []*MemoryStore{a, b} {
		grp := int64(10)
		if s == a {
			grp = -10
		}
		v, e := s.View(ctx, grp, 0, 0)
		if e != nil || len(v.Items) != 0 {
			t.Fatal("memory scope leaked")
		}
	}
	if e := a.Update(ctx, -10, 1, 1, "edit", "wrong scope"); e != ErrMemoryConflict {
		t.Fatalf("cross scope %v", e)
	}
	if e := a.Update(ctx, 10, 1, 1, "edit", "修订后的记忆"); e != nil {
		t.Fatal(e)
	}
	if e := a.Update(ctx, 10, 1, 1, "disable", ""); e != ErrMemoryConflict {
		t.Fatalf("stale version %v", e)
	}
	if e := a.Update(ctx, 10, 1, 2, "disable", ""); e != nil {
		t.Fatal(e)
	}
	loaded, e := a.LoadChecked(10)
	if e != nil || len(loaded) != 0 {
		t.Fatal("disabled memory loaded")
	}
	if e := a.Update(ctx, 10, 1, 3, "restore", ""); e != nil {
		t.Fatal(e)
	}
	loaded, e = a.LoadChecked(10)
	if e != nil || len(loaded) != 1 || !strings.Contains(loaded[0], "v4") {
		t.Fatal("restored version missing")
	}
	versions, e := a.View(ctx, 10, 0, 1)
	if e != nil || len(versions.Items) != 4 || versions.Items[0].Source != "agent" || versions.Items[1].Source != "admin" {
		t.Fatalf("revisions %v %+v", e, versions)
	}
	if e := a.Save(10, "line1\nline2"); e != ErrMemoryInput {
		t.Fatal("newline accepted")
	}
	// Read errors must propagate into the prompt, not masquerade as empty memory.
	if _, e = a.history.db.Exec("DROP TABLE agent_memories"); e != nil {
		t.Fatal(e)
	}
	if _, e = a.LoadChecked(10); e == nil {
		t.Fatal("read error hidden")
	}
}

func TestMemoryMigrationOwnershipIdempotencyAndRecovery(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	original := []byte("旧记忆一\r\n旧记忆二\n")
	if e := os.WriteFile(a.legacyPath(10), original, 0600); e != nil {
		t.Fatal(e)
	}
	v, e := a.View(ctx, 10, 0, 0)
	if e != nil || v.Mode != "legacy" || len(v.Items) != 0 {
		t.Fatal("unassigned plaintext exposed")
	}
	p, e := a.Preview(ctx, 10)
	if e != nil || p.Count != 2 {
		t.Fatalf("preview %v %+v", e, p)
	}
	if e := a.Migrate(ctx, 10, strings.Repeat("0", 64)); e != ErrMemoryConflict {
		t.Fatal("source change accepted")
	}
	if e := a.Migrate(ctx, 10, p.Digest); e != nil {
		t.Fatal(e)
	}
	if e := a.Migrate(ctx, 10, p.Digest); e != nil {
		t.Fatal(e)
	}
	v, e = a.View(ctx, 10, 0, 0)
	if e != nil || len(v.Items) != 2 || v.Items[0].Source != "legacy" || v.Items[0].SourceSeq != "0" {
		t.Fatal("migration duplicate or invented source")
	}
	if _, e := b.Preview(ctx, 10); e != ErrMemoryConflict {
		t.Fatal("other bot claimed same file")
	}
	if rows, e := b.LoadChecked(10); e != nil || len(rows) != 0 {
		t.Fatal("claimed legacy leaked")
	}
	if e := b.Save(10, "other bot independent"); e != nil {
		t.Fatal(e)
	}
	if e := a.Save(10, "new managed memory"); e != nil {
		t.Fatal(e)
	}
	old, _ := os.ReadFile(a.legacyPath(10))
	if string(old) != string(original) {
		t.Fatal("old file changed")
	}
	backups, e := filepath.Glob(filepath.Join(a.legacy.root, "10.*.bak"))
	if e != nil || len(backups) != 1 {
		t.Fatal("backup absent")
	}
	backup, _ := os.ReadFile(backups[0])
	if string(backup) != string(original) {
		t.Fatal("backup mismatch")
	}
	// A failed transaction must release its own reservation and keep the old file.
	if e := os.WriteFile(a.legacyPath(20), original, 0600); e != nil {
		t.Fatal(e)
	}
	p, e = a.Preview(ctx, 20)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.history.db.Exec("CREATE TRIGGER fail_memory BEFORE INSERT ON agent_memories BEGIN SELECT RAISE(ABORT,'fixture'); END"); e != nil {
		t.Fatal(e)
	}
	if e = a.Migrate(ctx, 20, p.Digest); e == nil {
		t.Fatal("injected failure passed")
	}
	if _, e = os.Stat(a.claimPath(20)); !os.IsNotExist(e) {
		t.Fatal("failed migration kept new claim")
	}
	if _, e = a.history.db.Exec("DROP TRIGGER fail_memory"); e != nil {
		t.Fatal(e)
	}
	if e = a.Migrate(ctx, 20, p.Digest); e != nil {
		t.Fatal(e)
	}
	// Simulate crash after reserving ownership; same bot can finish, others cannot.
	if e := os.WriteFile(a.legacyPath(30), original, 0600); e != nil {
		t.Fatal(e)
	}
	p, e = a.Preview(ctx, 30)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(a.claimPath(30), []byte(`{"bot":456,"digest":"`+p.Digest+`"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = a.LoadChecked(30); e != ErrMemoryConflict {
		t.Fatal("pending migration fell back")
	}
	if e = a.Migrate(ctx, 30, p.Digest); e != nil {
		t.Fatal(e)
	}
	// Reopening the account database retains managed ownership and revision data.
	if e = a.history.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := openAgentHistory(filepath.Join(filepath.Dir(a.legacy.root), "history"), a.bot, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	a.history = reopened
	v, e = a.View(ctx, 10, 0, 0)
	if e != nil || v.Mode != "managed" || len(v.Items) != 3 {
		t.Fatalf("restart lost memory: %v %+v", e, v)
	}
	if e = a.Migrate(ctx, 10, memoryDigest(original)); e != nil {
		t.Fatal("restart lost migration idempotency", e)
	}
}
