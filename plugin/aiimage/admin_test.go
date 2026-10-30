package aiimage

import (
	"errors"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/imagegen"
	sql "github.com/FloatTech/sqlite"
	"path/filepath"
	"testing"
)

func TestAdminSaveConflictBackupAndPrivacy(t *testing.T) {
	s := &storage{}
	if err := s.open(filepath.Join(t.TempDir(), "image.db")); err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	first, err := s.admin(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	next := &core.Update{Revision: first.Revision, Mode: "openai", URL: "http://127.0.0.1:8000/v1/images/generations", Model: "synthetic"}
	next.Key.Action = "replace"
	next.Key.Value = "synthetic-secret"
	backups := 0
	backup := func(*sql.Sqlite) error { backups++; return nil }
	saved, err := s.admin(next, backup)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.KeyConfigured || saved.Model != "synthetic" || saved.Mode != "openai" || backups != 1 {
		t.Fatal(saved)
	}
	if s.getConfig().APIKey != "synthetic-secret" {
		t.Fatal("key not stored")
	}
	if _, err = s.admin(next, backup); err == nil {
		t.Fatal("stale save accepted")
	}
	next.Revision = saved.Revision
	next.Key.Action = "keep"
	next.Key.Value = ""
	next.Model = "updated"
	if _, err = s.admin(next, func(*sql.Sqlite) error { return errors.New("failed") }); err == nil {
		t.Fatal("backup failure ignored")
	}
	if s.getConfig().ModelName != "synthetic" {
		t.Fatal("changed after failed backup")
	}
	saved, err = s.admin(next, backup)
	if err != nil {
		t.Fatal(err)
	}
	if s.getConfig().APIKey != "synthetic-secret" {
		t.Fatal("keep lost key")
	}
	next.Revision = saved.Revision
	next.Key.Action = "clear"
	if _, err = s.admin(next, backup); err == nil {
		t.Fatal("clear without confirmation")
	}
	next.Key.Confirm = true
	saved, err = s.admin(next, backup)
	if err != nil || saved.KeyConfigured {
		t.Fatal(saved, err)
	}
	// A private-chat edit invalidates an already loaded browser revision.
	if err = s.updateField("模型名", "chat-model"); err != nil {
		t.Fatal(err)
	}
	next.Revision = saved.Revision
	if _, err = s.admin(next, backup); err == nil {
		t.Fatal("chat conflict ignored")
	}
}
func TestAdminRejectsInvalidAndRedactsLegacyURL(t *testing.T) {
	s := &storage{}
	if err := s.open(filepath.Join(t.TempDir(), "image.db")); err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	for _, address := range []string{"file:///x", "https://user:pass@example.com/generate", "https://example.com/generate?key=secret"} {
		v, _ := s.admin(nil, nil)
		u := &core.Update{Revision: v.Revision, Mode: "openai", URL: address, Model: "x"}
		u.Key.Action = "keep"
		if _, err := s.admin(u, func(*sql.Sqlite) error { t.Fatal("invalid config backed up"); return nil }); err == nil {
			t.Fatal(address)
		}
	}
	if err := s.updateField("接口地址", "https://user:pass@example.com/generate?key=secret#secret"); err != nil {
		t.Fatal(err)
	}
	v, err := s.admin(nil, nil)
	if err != nil || v.URL != "https://example.com/generate" {
		t.Fatal(v, err)
	}
}
