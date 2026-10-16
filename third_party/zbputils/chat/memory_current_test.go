package chat

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestReadCurrentMemoryOutsidePageVersionAndScope(t *testing.T) {
	a, b := memoryFixture(t)
	ctx := context.Background()
	for i := 1; i <= 70; i++ {
		if err := a.Save(10, "memory "+strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Update(ctx, 10, 70, 1, "edit", "latest synthetic-agent-key 中文🙂"); err != nil {
		t.Fatal(err)
	}
	if err := a.Update(ctx, 10, 70, 2, "disable", ""); err != nil {
		t.Fatal(err)
	}
	list, err := a.View(ctx, 10, 0, 0)
	if err != nil || !list.More || len(list.Items) != 50 || list.Items[49].ID != "50" {
		t.Fatal(list, err)
	}
	for i := 0; i < 2; i++ {
		view, err := a.ReadCurrent(ctx, 10, 70)
		if err != nil || view.Mode != "managed" || len(view.Items) != 1 || view.More || view.Next != "" {
			t.Fatal(view, err)
		}
		item := view.Items[0]
		if item.ID != "70" || item.Revision != "3" || item.Active || item.Source != "admin" || strings.Contains(item.Text, "synthetic-agent-key") || !strings.Contains(item.Text, "中文🙂") {
			t.Fatal(item)
		}
	}
	versions, err := a.View(ctx, 10, 0, 70)
	if err != nil || len(versions.Items) != 3 {
		t.Fatal("read changed revision history", versions, err)
	}
	if err := a.Save(-10, "private memory"); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(10, "other bot memory"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		store   *MemoryStore
		grp, id int64
	}{{a, -10, 70}, {a, 11, 70}, {a, 10, 71}, {b, 10, 70}, {a, 10, 999}} {
		view, err := tc.store.ReadCurrent(ctx, tc.grp, tc.id)
		if err != nil || len(view.Items) != 0 {
			t.Fatal("current lookup crossed scope", tc, view, err)
		}
	}
	if _, err = a.ReadCurrent(ctx, 10, 0); !errors.Is(err, ErrMemoryInput) {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = a.ReadCurrent(cancelled, 10, 70); err == nil {
		t.Fatal("cancelled read succeeded")
	}
}

func TestReadCurrentMemoryDoesNotExposeLegacyText(t *testing.T) {
	a, _ := memoryFixture(t)
	if err := os.WriteFile(a.legacyPath(10), []byte("legacy private text"), 0600); err != nil {
		t.Fatal(err)
	}
	view, err := a.ReadCurrent(context.Background(), 10, 1)
	if err != nil || view.Mode != "legacy" || len(view.Items) != 0 {
		t.Fatal(view, err)
	}
}
