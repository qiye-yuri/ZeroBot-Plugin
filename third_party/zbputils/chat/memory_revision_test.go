package chat

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestMemoryViewRevisionScopeStateAndReadOnly(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	if e := a.Save(10, "old synthetic-agent-key 🙂"); e != nil {
		t.Fatal(e)
	}
	if e := a.Update(ctx, 10, 1, 1, "edit", "new 🙂"); e != nil {
		t.Fatal(e)
	}
	if e := a.Update(ctx, 10, 1, 2, "disable", ""); e != nil {
		t.Fatal(e)
	}
	before, e := a.ReadCurrent(ctx, 10, 1)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		revision int64
		text     string
		active   bool
	}{{1, "old [已隐藏] 🙂", true}, {2, "new 🙂", true}, {3, "new 🙂", false}} {
		p, e := a.ViewRevision(ctx, 10, 1, tc.revision)
		if e != nil || p.Mode != "managed" || len(p.Items) != 1 || p.More {
			t.Fatal(tc, p, e)
		}
		item := p.Items[0]
		if item.Text != tc.text || item.Active != tc.active || item.ID != "1" || strings.Contains(item.Text, "synthetic-agent-key") {
			t.Fatal(item)
		}
	}
	for _, tc := range []struct {
		s                 *MemoryStore
		grp, id, revision int64
	}{{a, -10, 1, 1}, {b, 10, 1, 1}, {a, 10, 2, 1}, {a, 10, 1, 4}} {
		p, e := tc.s.ViewRevision(ctx, tc.grp, tc.id, tc.revision)
		if e != nil || len(p.Items) != 0 {
			t.Fatal("scope or missing version", p, e)
		}
	}
	for _, scope := range [][3]int64{{0, 1, 1}, {10, 0, 1}, {10, -1, 1}, {10, 1, 0}, {10, 1, -1}} {
		if _, e := a.ViewRevision(ctx, scope[0], scope[1], scope[2]); !errors.Is(e, ErrMemoryInput) {
			t.Fatal(scope, e)
		}
	}
	after, e := a.ReadCurrent(ctx, 10, 1)
	if e != nil {
		t.Fatal(e)
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if string(beforeJSON) != string(afterJSON) {
		t.Fatal("read changed current memory")
	}
	var count int
	if e := a.history.db.QueryRow(`SELECT count(*) FROM agent_memory_revisions WHERE memory_id=1`).Scan(&count); e != nil || count != 3 {
		t.Fatal(count, e)
	}
	for _, revision := range []int64{1, 3} {
		p, e := a.ReadEnabled(ctx, 10, 1, revision)
		if e != nil || len(p.Memories) != 0 {
			t.Fatal("model gained old or disabled access", p, e)
		}
	}
	if e := os.WriteFile(a.legacyPath(20), []byte("legacy text"), 0600); e != nil {
		t.Fatal(e)
	}
	p, e := a.ViewRevision(ctx, 20, 1, 1)
	if e != nil || p.Mode != "legacy" || len(p.Items) != 0 {
		t.Fatal(p, e)
	}
}

func TestMemoryViewRevisionPrecisionCancellationAndFailure(t *testing.T) {
	a, _ := memoryFixture(t)
	ctx := context.Background()
	if e := a.Save(10, "current"); e != nil {
		t.Fatal(e)
	}
	if _, e := a.history.db.Exec(`INSERT INTO agent_memory_revisions(memory_id,revision,text,active,source,source_seq,updated_at) VALUES(1,9007199254740993,'exact old',0,'admin',0,'fixture'),(1,9007199254740994,'exact current',1,'admin',0,'fixture')`); e != nil {
		t.Fatal(e)
	}
	if _, e := a.history.db.Exec(`UPDATE agent_memories SET revision=9007199254740994,text='exact current' WHERE id=1`); e != nil {
		t.Fatal(e)
	}
	p, e := a.ViewRevision(ctx, 10, 1, 9007199254740993)
	if e != nil || len(p.Items) != 1 || p.Items[0].Revision != "9007199254740993" || p.Items[0].Text != "exact old" || p.Items[0].Active {
		t.Fatal(p, e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := a.ViewRevision(cancelled, 10, 1, 1); e == nil {
		t.Fatal("cancel ignored")
	}
	if _, e := a.history.db.Exec(`DROP TABLE agent_memory_revisions`); e != nil {
		t.Fatal(e)
	}
	if _, e := a.ViewRevision(ctx, 10, 1, 1); !errors.Is(e, ErrMemory) {
		t.Fatal("storage failure looked missing", e)
	}
}
