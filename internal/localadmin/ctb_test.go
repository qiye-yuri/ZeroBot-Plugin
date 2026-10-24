package localadmin

import (
	"encoding/json"
	"github.com/FloatTech/ZeroBot-Plugin/internal/ctb"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCTBEndpointAuthorization(t *testing.T) {
	s := newTestServer(t)
	expectStatus(t, request(s, "GET", "/api/ctb", nil, nil, ""), 401)
	cookie, csrf := enroll(t, s)
	expectStatus(t, request(s, "GET", "/api/ctb", nil, cookie, ""), 200)
	expectStatus(t, request(s, "POST", "/api/ctb", map[string]any{"revision": strings.Repeat("a", 64)}, cookie, "bad"), 403)
	expectStatus(t, request(s, "POST", "/api/ctb", map[string]any{}, cookie, csrf), 400)
}

func TestCTBStatusReadPreservesFilesAndRevisionConflict(t *testing.T) {
	dir := t.TempDir()
	old := ctb.Default
	ctb.Default = &ctb.Service{Path: filepath.Join(dir, "config.json")}
	t.Cleanup(func() { ctb.Default = old })
	config := ctb.Defaults()
	config.Executable = filepath.Join(dir, "not-started.exe")
	config.OutputDir = filepath.Join(dir, "downloads")
	config.Enabled = true
	writeConfig := func() {
		b, e := json.Marshal(config)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(ctb.Default.Path, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	writeConfig()
	if e := os.Mkdir(filepath.Join(dir, "backups"), 0700); e != nil {
		t.Fatal(e)
	}
	for name, body := range map[string]string{"backups/old.json": "old backup", "group-usage.json": "{\"date\":\"2026-10-24\",\"counts\":{\"123\":2}}"} {
		if e := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
	}
	snapshot := func() map[string]string {
		values := map[string]string{}
		e := filepath.WalkDir(dir, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				return nil
			}
			b, e := os.ReadFile(p)
			values[p] = string(b)
			return e
		})
		if e != nil {
			t.Fatal(e)
		}
		return values
	}
	s := newTestServer(t)
	cookie, csrf := enroll(t, s)
	read := func() ctb.Snapshot {
		w := request(s, "GET", "/api/ctb", nil, cookie, "")
		expectStatus(t, w, 200)
		var v ctb.Snapshot
		if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	before := snapshot()
	first := read()
	second := read()
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("GET changed state or files")
	}
	config.Threads++
	writeConfig()
	changed := snapshot()
	fresh := read()
	if fresh.Revision == first.Revision {
		t.Fatal("fixture did not change revision")
	}
	expectStatus(t, request(s, "POST", "/api/ctb", map[string]any{"revision": first.Revision, "config": first.Config}, cookie, csrf), 409)
	if !reflect.DeepEqual(changed, snapshot()) {
		t.Fatal("status GET or rejected stale save wrote files")
	}
}
