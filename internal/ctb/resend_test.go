package ctb

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
)

func resendFixture(t *testing.T) (*Service, Config, string, string) {
	t.Helper()
	root := t.TempDir()
	s := &Service{Path: filepath.Join(root, "config.json")}
	c := Defaults()
	c.Enabled, c.Upload, c.GroupAccess = true, true, "all"
	c.OutputDir = filepath.Join(root, "downloads")
	task := "ctb-135-12345"
	dir := filepath.Join(c.OutputDir, task)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	archive := archiveFixture(t, "manifest.json")
	b, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "pack-windows.zip")
	if err = os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err = recordDelivery(dir, []string{p}); err != nil {
		t.Fatal(err)
	}
	if err = core.Save(s.Path, c); err != nil {
		t.Fatal(err)
	}
	return s, c, task, p
}

func TestResendUsesRecordedFilesAndPreservesQuota(t *testing.T) {
	s, c, task, file := resendFixture(t)
	if err := s.reserveGroupDownload(101, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	quota := filepath.Join(filepath.Dir(s.Path), "group-usage.json")
	before, err := os.ReadFile(quota)
	if err != nil {
		t.Fatal(err)
	}
	// An original ZIP that was not selected for delivery must not be resent.
	if err = os.WriteFile(filepath.Join(filepath.Dir(file), "original.zip"), []byte("not selected"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		group             int64
		mode, action, key string
	}{
		{101, "group", "upload_group_file", "group_id"},
		{101, "private", "upload_private_file", "user_id"},
		{0, "group", "upload_private_file", "user_id"},
	} {
		c.GroupDelivery = tc.mode
		if err = core.Save(s.Path, c); err != nil {
			t.Fatal(err)
		}
		restarted := &Service{Path: s.Path}
		snapshot, err := restarted.Settings("", nil)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		err = restarted.Resend(task, 123, tc.group, func(action string, params map[string]any) error {
			calls++
			target := int64(123)
			if tc.key == "group_id" {
				target = tc.group
			}
			if action != tc.action || params[tc.key] != target || params["file"] != file {
				t.Fatalf("unexpected delivery: %s %+v", action, params)
			}
			if _, err := restarted.Settings(snapshot.Revision, &c); err == nil {
				t.Fatal("settings changed during resend")
			}
			return nil
		})
		if err != nil || calls != 1 || restarted.busy {
			t.Fatal(err, calls, restarted.busy)
		}
	}
	after, err := os.ReadFile(quota)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("resend changed download quota", err)
	}
}

func TestResendRejectsMissingChangedAndUnrecordedFiles(t *testing.T) {
	for _, mode := range []string{"invalid-id", "legacy", "changed", "missing", "manifest-path", "disabled", "upload-disabled", "busy"} {
		t.Run(mode, func(t *testing.T) {
			s, c, task, file := resendFixture(t)
			switch mode {
			case "invalid-id":
				task = "../ctb-135-12345"
			case "legacy":
				if err := os.Remove(filepath.Join(filepath.Dir(file), "delivery.json")); err != nil {
					t.Fatal(err)
				}
			case "changed":
				if err := os.WriteFile(file, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
			case "manifest-path":
				if err := os.WriteFile(filepath.Join(filepath.Dir(file), "delivery.json"), []byte(`[{"name":"../outside.zip","sha256":"invalid"}]`), 0600); err != nil {
					t.Fatal(err)
				}
			case "disabled":
				c.Enabled = false
			case "upload-disabled":
				c.Upload = false
			case "busy":
				s.busy = true
			}
			if err := core.Save(s.Path, c); err != nil {
				t.Fatal(err)
			}
			if err := s.Resend(task, 123, 101, func(string, map[string]any) error { t.Fatal("invalid resend sent a file"); return nil }); err == nil {
				t.Fatal("invalid resend accepted")
			}
		})
	}
}

func TestResendFailureReleasesBusyAndCanRetry(t *testing.T) {
	s, _, task, _ := resendFixture(t)
	want := errors.New("synthetic upload failure")
	if err := s.Resend(task, 123, 0, func(string, map[string]any) error { return want }); !errors.Is(err, want) || s.busy {
		t.Fatal(err, s.busy)
	}
	if err := s.Resend(task, 123, 0, func(string, map[string]any) error { return nil }); err != nil {
		t.Fatal(err)
	}
}
