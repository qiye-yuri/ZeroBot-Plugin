package chat

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestLayeredPersonalRevisionIsolationAndSharedKnowledge(t *testing.T) {
	a, b := memoryFixture(t)
	seq, e := a.history.appendWithSequence(10, "event", historyEvent(10, "我现在喜欢红茶"))
	if e != nil {
		t.Fatal(e)
	}
	spec := MemorySpec{Kind: "personal", Subject: 123, Attribute: "preference.tea"}
	saved, e := a.SaveLayered(10, "用户喜欢红茶", seq, spec, false)
	if e != nil || !*saved.Created {
		t.Fatal(saved, e)
	}
	repeated, e := a.SaveLayered(10, "用户喜欢红茶", seq, spec, false)
	if e != nil || *repeated.Created || repeated.Memory.ID != saved.Memory.ID {
		t.Fatal(repeated, e)
	}
	if _, e = a.SaveLayered(10, "用户改喝绿茶", seq, spec, false); !errors.Is(e, ErrMemoryConflict) {
		t.Fatal("missing revision accepted", e)
	}
	spec.Revision = 1
	updated, e := a.SaveLayered(10, "用户改喝绿茶", seq, spec, false)
	if e != nil || updated.Memory.Revision != "2" {
		t.Fatal(updated, e)
	}
	if _, e = a.SaveLayered(10, "stale", seq, spec, false); !errors.Is(e, ErrMemoryConflict) {
		t.Fatal("stale revision", e)
	}
	spec.Subject = 999
	if _, e = a.SaveLayered(10, "unrelated person", seq, spec, false); !errors.Is(e, ErrMemoryInput) {
		t.Fatal("invented subject", e)
	}
	for _, s := range []*MemoryStore{a, b} {
		grp := int64(20)
		if s == b {
			grp = 10
		}
		lines, e := s.LoadChecked(grp)
		if e != nil || len(lines) != 0 {
			t.Fatal("private leakage", lines, e)
		}
	}
	shared := MemorySpec{Kind: "permanent", Attribute: "style"}
	if _, e = a.SaveLayered(10, "喜欢简洁措辞", seq, shared, false); !errors.Is(e, ErrMemoryInput) {
		t.Fatal("unauthorized shared", e)
	}
	saved, e = a.SaveLayered(10, "喜欢简洁措辞", seq, shared, true)
	if e != nil {
		t.Fatal(e)
	}
	lines, e := a.LoadChecked(20)
	if e != nil || len(lines) != 1 || !strings.Contains(lines[0], "简洁") {
		t.Fatal(lines, e)
	}
	lines, e = b.LoadChecked(20)
	if e != nil || len(lines) != 0 {
		t.Fatal("other bot", lines, e)
	}
	id, _ := strconv.ParseInt(saved.Memory.ID, 10, 64)
	read, e := a.ReadEnabled(context.Background(), 20, id, 1)
	if e != nil || len(read.Memories) != 1 || read.Memories[0].Kind != "permanent" || read.Memories[0].SourceSeq != "0" {
		t.Fatal("shared read provenance", read, e)
	}
	search, e := a.SearchEnabled(context.Background(), 20, "简洁", 5)
	if e != nil || len(search.Memories) != 1 || search.Memories[0].Attribute != "style" || search.Memories[0].SourceSeq != "0" {
		t.Fatal("shared search", search, e)
	}
	if e = a.Update(context.Background(), 10, id, 1, "disable", ""); e != nil {
		t.Fatal(e)
	}
	lines, e = a.LoadChecked(20)
	if e != nil || len(lines) != 0 {
		t.Fatal("disabled shared", lines, e)
	}
}

func TestLayeredRestoreCannotReviveConflictingAttribute(t *testing.T) {
	a, _ := memoryFixture(t)
	seq, _ := a.history.appendWithSequence(10, "event", historyEvent(10, "nickname"))
	spec := MemorySpec{Kind: "personal", Subject: 123, Attribute: "nickname"}
	saved, e := a.SaveLayered(10, "old name", seq, spec, false)
	if e != nil {
		t.Fatal(e)
	}
	id, _ := strconv.ParseInt(saved.Memory.ID, 10, 64)
	if e = a.Update(context.Background(), 10, id, 1, "disable", ""); e != nil {
		t.Fatal(e)
	}
	if _, e = a.SaveLayered(10, "new name", seq, spec, false); e != nil {
		t.Fatal(e)
	}
	if e = a.Update(context.Background(), 10, id, 2, "restore", ""); !errors.Is(e, ErrMemoryConflict) {
		t.Fatal("conflicting active attribute", e)
	}
}

func TestMemoryRecallFindsOlderRelatedFact(t *testing.T) {
	a, _ := memoryFixture(t)
	if e := a.Save(10, "古老的重要记录：周末一起去爬山"); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 110; i++ {
		if e := a.Save(10, fmt.Sprintf("无关信息%d", i)); e != nil {
			t.Fatal(e)
		}
	}
	if e := a.history.Append(10, "event", historyEvent(10, "周末爬山还有安排吗")); e != nil {
		t.Fatal(e)
	}
	lines, e := a.LoadChecked(10)
	if e != nil || len(lines) != 100 || !strings.Contains(strings.Join(lines, "\n"), "古老的重要记录") {
		t.Fatal("relevant old memory not recalled", e)
	}
}

func TestLayeredFailureRollsBackTextAndMetadata(t *testing.T) {
	a, _ := memoryFixture(t)
	seq, _ := a.history.appendWithSequence(10, "event", historyEvent(10, "facts"))
	_, e := a.history.db.Exec(`CREATE TRIGGER reject_layer BEFORE INSERT ON agent_memory_layers BEGIN SELECT RAISE(ABORT,'fixture'); END`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = a.SaveLayered(10, "fact", seq, MemorySpec{Kind: "personal", Subject: 123, Attribute: "nickname"}, false)
	if e == nil {
		t.Fatal("write accepted")
	}
	var n int
	if e = a.history.db.QueryRow(`SELECT COUNT(*) FROM agent_memories`).Scan(&n); e != nil || n != 0 {
		t.Fatal("partial save", n, e)
	}
}
