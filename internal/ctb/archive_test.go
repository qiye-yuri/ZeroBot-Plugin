package ctb

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func archiveFixture(t *testing.T, names ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pack.zip")
	f, e := os.Create(p)
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(f)
	for _, n := range names {
		w, e := z.Create(n)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte("payload:" + n)); e != nil {
			t.Fatal(e)
		}
	}
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	f.Close()
	return p
}

func TestPrepareArchiveRepairsPathsWithoutChangingPayload(t *testing.T) {
	p := archiveFixture(t, "overrides//default-server.properties", "modrinth.index.json")
	original, _ := os.ReadFile(p)
	fixed, n, e := PrepareArchive(p)
	if e != nil || n != 1 || fixed == p {
		t.Fatalf("repair: %s %d %v", fixed, n, e)
	}
	after, _ := os.ReadFile(p)
	if string(after) != string(original) {
		t.Fatal("original changed")
	}
	z, e := zip.OpenReader(fixed)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	if len(z.File) != 2 || z.File[0].Name != "overrides/default-server.properties" {
		t.Fatal("unexpected entries")
	}
	r, e := z.File[0].Open()
	if e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(r)
	r.Close()
	if e != nil || string(b) != "payload:overrides//default-server.properties" {
		t.Fatal("payload changed", e)
	}
	if got, n, e := PrepareArchive(fixed); e != nil || n != 0 || got != fixed {
		t.Fatal("not idempotent", got, n, e)
	}
}

func TestPrepareArchiveRejectsUnsafePathsAndCollisions(t *testing.T) {
	for _, names := range [][]string{{"../evil"}, {"/root"}, {"a/../evil"}, {"a/NUL.txt"}, {"a/b:ads"}, {"a/trailing. "}, {"a//b", "a/b"}, {"a/B", "a/b"}, {"a", "a/b"}} {
		if _, _, e := PrepareArchive(archiveFixture(t, names...)); e == nil {
			t.Fatalf("accepted %v", names)
		}
	}
}
