package ctb

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestGroupPolicyAndDelivery(t *testing.T) {
	for _, v := range []struct {
		group, at, owner, admin bool
		access, action          string
		want                    bool
	}{
		{false, false, true, false, "disabled", "download", true},
		{false, false, false, false, "all", "download", false},
		{true, false, true, true, "all", "download", false},
		{true, true, false, false, "all", "download", true},
		{true, true, true, true, "disabled", "search", false},
		{true, true, false, false, "admin", "search", true},
		{true, true, false, false, "admin", "download", false},
		{true, true, false, true, "admin", "download", true},
		{true, true, false, true, "owner", "search", false},
	} {
		if CanUse(v.group, v.at, v.owner, v.admin, v.access, v.action) != v.want {
			t.Fatalf("policy %+v", v)
		}
	}
	for _, v := range []struct {
		group             int64
		mode, action, key string
		target            int64
	}{
		{999, "group", "upload_group_file", "group_id", 999},
		{999, "private", "upload_private_file", "user_id", 123},
		{0, "group", "upload_private_file", "user_id", 123},
	} {
		err := Deliver([]string{filepath.Join(t.TempDir(), "pack.zip")}, 123, v.group, v.mode, func(action string, p map[string]any) error {
			if action != v.action || p[v.key] != v.target || p["name"] != "pack.zip" || len(p) != 3 {
				t.Fatalf("wrong destination: %s %+v", action, p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	want := errors.New("denied")
	if err := Deliver([]string{"pack.zip"}, 123, 999, "group", func(string, map[string]any) error { return want }); !errors.Is(err, want) {
		t.Fatal("upload failure hidden")
	}
}

func TestGroupQuotaPersistenceAndMidnight(t *testing.T) {
	s := &Service{Path: filepath.Join(t.TempDir(), "config.json")}
	now := time.Date(2026, 10, 24, 15, 59, 0, 0, time.UTC) // 23:59 Beijing
	for i := 0; i < 5; i++ {
		if err := s.reserveGroupDownload(101, 5, now); err != nil {
			t.Fatal(err)
		}
	}
	restarted := &Service{Path: s.Path}
	if err := restarted.reserveGroupDownload(101, 5, now); err == nil {
		t.Fatal("restart bypassed quota")
	}
	if err := restarted.reserveGroupDownload(102, 5, now); err != nil {
		t.Fatal("group isolation", err)
	}
	if err := restarted.reserveGroupDownload(101, 5, now.Add(time.Minute)); err != nil {
		t.Fatal("midnight reset", err)
	}
}
