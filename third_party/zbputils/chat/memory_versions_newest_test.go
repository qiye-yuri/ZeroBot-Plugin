package chat

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestMemoryVersionsNewestPagingAndCompatibility(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	if err := a.Save(10, "revision 1 synthetic-agent-key 🙂"); err != nil {
		t.Fatal(err)
	}
	for revision := int64(2); revision <= 121; revision++ {
		if err := a.Update(ctx, 10, 1, revision-1, "edit", fmt.Sprintf("revision %d synthetic-agent-key 🙂", revision)); err != nil {
			t.Fatal(err)
		}
	}
	before, expected := int64(0), int64(121)
	for pageNo := 0; pageNo < 3; pageNo++ {
		page, err := a.ViewVersionsNewest(ctx, 10, 1, before)
		want := 50
		if pageNo == 2 {
			want = 21
		}
		if err != nil || len(page.Items) != want || page.More != (pageNo < 2) {
			t.Fatal(pageNo, page, err)
		}
		for _, item := range page.Items {
			if item.ID != "1" || item.Revision != strconv.FormatInt(expected, 10) || strings.Contains(item.Text, "synthetic-agent-key") || !item.Active {
				t.Fatal("revision order, content or state changed", item, expected)
			}
			expected--
		}
		before, _ = strconv.ParseInt(page.Next, 10, 64)
		if pageNo == 0 {
			if err := a.Update(ctx, 10, 1, 121, "disable", ""); err != nil {
				t.Fatal(err)
			}
		}
	}
	if expected != 0 {
		t.Fatal("missing or repeated revision", expected)
	}
	fresh, err := a.ViewVersionsNewest(ctx, 10, 1, 0)
	if err != nil || fresh.Items[0].Revision != "122" || fresh.Items[0].Active {
		t.Fatal("fresh history missed latest disabled revision", fresh, err)
	}
	old, err := a.View(ctx, 10, 0, 1)
	if err != nil || len(old.Items) != 50 || old.Items[0].Revision != "1" || old.Next != "50" || !old.More {
		t.Fatal("old ascending API changed", old, err)
	}
	for _, scope := range []struct {
		store *MemoryStore
		grp   int64
		id    int64
	}{{a, -10, 1}, {a, 11, 1}, {a, 10, 2}, {b, 10, 1}} {
		page, err := scope.store.ViewVersionsNewest(ctx, scope.grp, scope.id, 0)
		if err != nil || len(page.Items) != 0 {
			t.Fatal("revision scope leaked", scope, page, err)
		}
	}
	page, err := a.ViewVersionsNewest(ctx, 10, 1, 1)
	if err != nil || len(page.Items) != 0 || page.More {
		t.Fatal("exclusive lower boundary ignored", page, err)
	}
	for _, args := range [][3]int64{{0, 1, 0}, {10, 0, 0}, {10, 1, -1}} {
		if _, err := a.ViewVersionsNewest(ctx, args[0], args[1], args[2]); err != ErrMemoryInput {
			t.Fatal(args, err)
		}
	}
	var count int
	if err := a.history.db.QueryRow(`SELECT count(*) FROM agent_memory_revisions WHERE memory_id=1`).Scan(&count); err != nil || count != 122 {
		t.Fatal("reading changed history", count, err)
	}
	read, err := a.ReadEnabled(ctx, 10, 1, 122)
	if err != nil || len(read.Memories) != 0 {
		t.Fatal("management history enabled a disabled memory", read, err)
	}
}

func TestMemoryVersionsNewestLargeRevisionBoundary(t *testing.T) {
	a, _ := memoryFixture(t)
	if err := a.Save(10, "large revision"); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []int64{9007199254740993, 9007199254740994} {
		if _, err := a.history.db.Exec(`INSERT INTO agent_memory_revisions(memory_id,revision,text,active,source,source_seq,updated_at) VALUES(1,?,'large revision',1,'agent',0,'fixture')`, revision); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.history.db.Exec(`UPDATE agent_memories SET revision=9007199254740994 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	page, err := a.ViewVersionsNewest(context.Background(), 10, 1, 9007199254740994)
	if err != nil || len(page.Items) != 2 || page.Items[0].Revision != "9007199254740993" || page.Items[1].Revision != "1" {
		t.Fatal(page, err)
	}
}
