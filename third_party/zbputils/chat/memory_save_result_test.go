package chat

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestMemorySaveResultIdentityReuseAndScope(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	if _, e := a.history.db.Exec(`INSERT INTO history(seq,conversation,kind,payload,created_at) VALUES(9007199254740993,10,'event','{}','fixture')`); e != nil {
		t.Fatal(e)
	}
	if _, e := a.history.db.Exec(`INSERT INTO sqlite_sequence(name,seq) VALUES('agent_memories',9007199254740993)`); e != nil {
		t.Fatal(e)
	}
	const seq = int64(9007199254740993)
	saved, e := a.SaveFromSequenceResult(10, "  metadata body synthetic-agent-key  ", seq)
	if e != nil || saved.Mode != "managed" || saved.Created == nil || !*saved.Created || saved.Memory == nil || saved.Memory.ID != "9007199254740994" || saved.Memory.Revision != "1" || saved.Memory.Source != "agent" || saved.Memory.SourceSeq != "9007199254740993" {
		t.Fatal(e, saved)
	}
	ref := *saved.Memory
	raw, _ := json.Marshal(saved)
	if strings.Contains(string(raw), "metadata body") || strings.Contains(string(raw), "synthetic-agent-key") || strings.Contains(string(raw), `"text"`) {
		t.Fatal("save result retained text", string(raw))
	}
	view, e := a.View(ctx, 10, 0, 0)
	if e != nil || len(view.Items) != 1 {
		t.Fatal(e, view)
	}
	original := view.Items[0]
	// Returned pointers must not mutate stored metadata.
	saved.Memory.ID = "wrong"
	*saved.Created = false
	second, e := a.history.appendWithSequence(10, "event", historyEvent(10, "later trigger"))
	if e != nil {
		t.Fatal(e)
	}
	reused, e := a.SaveFromSequenceResult(10, original.Text, second)
	if e != nil || reused.Created == nil || *reused.Created || reused.Memory == nil || *reused.Memory != ref {
		t.Fatal("dedup changed original reference", e, reused)
	}
	view, e = a.View(ctx, 10, 0, 0)
	if e != nil || view.Items[0] != original {
		t.Fatal("dedup metadata changed", e, view)
	}
	id, _ := strconv.ParseInt(ref.ID, 10, 64)
	if e = a.Update(ctx, 10, id, 1, "edit", "admin body"); e != nil {
		t.Fatal(e)
	}
	reused, e = a.SaveFromSequenceResult(10, "admin body", second)
	if e != nil || *reused.Created || reused.Memory.ID != ref.ID || reused.Memory.Revision != "2" || reused.Memory.Source != "admin" || reused.Memory.SourceSeq != "0" {
		t.Fatal("admin reference was overwritten", e, reused)
	}
	old, e := a.ReadEnabled(ctx, 10, id, 1)
	if e != nil || len(old.Memories) != 0 {
		t.Fatal("returned reference reserved stale version", e, old)
	}
	if e = a.Update(ctx, 10, id, 2, "disable", ""); e != nil {
		t.Fatal(e)
	}
	fresh, e := a.SaveFromSequenceResult(10, "admin body", seq)
	if e != nil || !*fresh.Created || fresh.Memory.ID == ref.ID || fresh.Memory.SourceSeq != ref.SourceSeq {
		t.Fatal("disabled record was reused", e, fresh)
	}
	for _, scope := range []struct {
		store *MemoryStore
		grp   int64
	}{{a, -10}, {b, 10}} {
		separate, e := scope.store.SaveFromSequenceResult(scope.grp, "admin body", 0)
		if e != nil || !*separate.Created || separate.Memory.ID == fresh.Memory.ID || separate.Memory.SourceSeq != "0" {
			t.Fatal("reference crossed scope", e, separate)
		}
	}
	if result, e := a.SaveFromSequenceResult(-10, "admin body", seq); e != ErrMemoryConflict || result.Memory != nil || result.Created != nil || result.Mode != "" {
		t.Fatal("invalid source returned a reference", e, result)
	}
}

func TestMemorySaveResultConcurrentReuseAndAtomicFailure(t *testing.T) {
	a, _ := memoryFixture(t)
	var wg sync.WaitGroup
	results := make(chan MemorySaveResult, 24)
	failures := make(chan error, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := a.SaveFromSequenceResult(10, "same body", 0)
			results <- r
			failures <- e
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	created := 0
	for r := range results {
		if r.Mode != "managed" || r.Created == nil || r.Memory == nil || r.Memory.ID != "1" || r.Memory.Revision != "1" || r.Memory.SourceSeq != "0" {
			t.Fatal(r)
		}
		if *r.Created {
			created++
		}
	}
	if created != 1 {
		t.Fatal("concurrent calls reported multiple new records", created)
	}
	v, e := a.View(context.Background(), 10, 0, 1)
	if e != nil || len(v.Items) != 1 {
		t.Fatal("duplicate added revision", e, v)
	}
	if _, e = a.history.db.Exec(`CREATE TRIGGER reject_save_result_revision BEFORE INSERT ON agent_memory_revisions BEGIN SELECT RAISE(ABORT,'fixture'); END`); e != nil {
		t.Fatal(e)
	}
	r, e := a.SaveFromSequenceResult(10, "atomic new body", 0)
	if e != ErrMemory || r.Memory != nil || r.Created != nil || r.Mode != "" {
		t.Fatal("failed transaction returned success metadata", e, r)
	}
	v, e = a.View(context.Background(), 10, 0, 0)
	if e != nil || len(v.Items) != 1 {
		t.Fatal("failed transaction left memory", e, v)
	}
	if _, e = a.history.db.Exec(`UPDATE agent_memories SET revision=0 WHERE id=1`); e != nil {
		t.Fatal(e)
	}
	r, e = a.SaveFromSequenceResult(10, "same body", 0)
	if e != ErrMemory || r.Memory != nil || r.Created != nil {
		t.Fatal("corrupt reference reported reusable", e, r)
	}
}

func TestMemorySaveResultLegacyOmitsFabricatedIdentity(t *testing.T) {
	a, _ := memoryFixture(t)
	if e := os.WriteFile(a.legacyPath(10), []byte("legacy initial\n"), 0600); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		r, e := a.SaveFromSequenceResult(10, "legacy body synthetic-agent-key", 0)
		if e != nil || r.Mode != "legacy" || r.Created != nil || r.Memory != nil {
			t.Fatal("legacy record identity invented", e, r)
		}
		raw, _ := json.Marshal(r)
		if string(raw) != `{"mode":"legacy"}` {
			t.Fatal(string(raw))
		}
	}
	data, e := os.ReadFile(a.legacyPath(10))
	if e != nil || strings.Contains(string(data), "synthetic-agent-key") || strings.Count(string(data), "legacy body") != 2 {
		t.Fatal("legacy append or redaction changed", e)
	}
	a.history.config = func() (AgentConfig, error) { return AgentConfig{}, errors.New("fixture unavailable") }
	r, e := a.SaveFromSequenceResult(10, "cannot persist", 0)
	if e != ErrMemory || r.Memory != nil || r.Created != nil || r.Mode != "" {
		t.Fatal("legacy failure returned metadata", e, r)
	}
	after, e := os.ReadFile(a.legacyPath(10))
	if e != nil || string(after) != string(data) {
		t.Fatal("failed legacy save changed file", e)
	}
}
