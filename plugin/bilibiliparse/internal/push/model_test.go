package push

import (
	"path/filepath"
	"testing"

	"github.com/jinzhu/gorm"
	_ "modernc.org/sqlite"
)

func TestAtAllPersistenceAndGroupIsolation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "push.db")
	db := initializePush(p)
	defer func() { _ = (*gorm.DB)(db).Close() }()
	if got := db.getAtAll(101); got != 0 {
		t.Fatalf("default=%d", got)
	}
	// A real subscription must not shadow the separate notification settings.
	if err := db.insertOrUpdateLiveAndDynamic(map[string]any{"bilibili_uid": 9, "group_id": 101}); err != nil {
		t.Fatal(err)
	}
	for _, value := range []int64{1, 0, 1} {
		if err := db.updateAtAll(map[string]any{"group_id": 101, "at_all": value}); err != nil {
			t.Fatal(err)
		}
		if got := db.getAtAll(101); got != value {
			t.Fatalf("saved=%d read=%d", value, got)
		}
		if got := db.getAtAll(102); got != 0 {
			t.Fatalf("other group changed: %d", got)
		}
	}
	if err := (*gorm.DB)(db).Close(); err != nil {
		t.Fatal(err)
	}
	db = initializePush(p)
	if got := db.getAtAll(101); got != 1 {
		t.Fatalf("reopened=%d", got)
	}
}

func TestSubscriptionChannelsAndSessions(t *testing.T) {
	db := initializePush(filepath.Join(t.TempDir(), "push.db"))
	defer (*gorm.DB)(db).Close()
	put := func(gid int64, fields map[string]any) {
		t.Helper()
		fields["bilibili_uid"], fields["group_id"] = int64(9), gid
		if err := db.insertOrUpdateLiveAndDynamic(fields); err != nil {
			t.Fatal(err)
		}
	}
	put(101, map[string]any{"live_disable": 0, "dynamic_disable": 0})
	put(-101, map[string]any{"live_disable": 0, "dynamic_disable": 0})
	put(101, map[string]any{"dynamic_disable": 1})
	if got := db.getAllGroupByBuidAndDynamic(9); len(got) != 1 || got[0] != -101 {
		t.Fatalf("dynamic targets=%v", got)
	}
	if got := db.getAllGroupByBuidAndLive(9); len(got) != 2 {
		t.Fatalf("live targets=%v", got)
	}
	if got := db.getAllBuidByLive(); len(got) != 1 || got[0] != 9 {
		t.Fatalf("duplicate UIDs=%v", got)
	}
	put(101, map[string]any{"live_disable": 1})
	if got := db.getAllPushByGroup(101); len(got) != 0 {
		t.Fatalf("disabled subscription visible=%v", got)
	}
	if got := db.getAllPushByGroup(-101); len(got) != 1 {
		t.Fatalf("private subscription changed=%v", got)
	}
	put(101, map[string]any{"live_disable": 0, "dynamic_disable": 0})
	put(101, map[string]any{"live_disable": 0, "dynamic_disable": 0})
	if got := db.getAllPushByGroup(101); len(got) != 1 {
		t.Fatalf("resubscribe duplicated=%v", got)
	}
}
