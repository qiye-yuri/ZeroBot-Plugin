package resourcecheck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func put(t *testing.T, root, name string, b []byte) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 3)), nil); err != nil {
		t.Fatal(err)
	}
	put(t, root, "Tarot/tarots.json", []byte(`{"a":{"info":{"imgUrl":"a.png"}}}`))
	put(t, root, "Tarot/a.png", b.Bytes()) // Deliberately JPEG with PNG extension.
	put(t, root, "Wife/wife.json", []byte(`["broken.png","directory.png","broken.png"]`))
	put(t, root, "Wife/wives/broken.png", []byte("not an image"))
	if err := os.MkdirAll(filepath.Join(root, "Wife/wives/directory.png"), 0755); err != nil {
		t.Fatal(err)
	}
	put(t, root, "nwife/private.png", []byte("unrelated user file"))
	return root
}
func hashes(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			out[p] = sha256.Sum256(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestInspectReadOnlyAndClassification(t *testing.T) {
	root := fixture(t)
	before := hashes(t, root)
	r, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	a, b := r.Groups[0], r.Groups[1]
	if a.Expected != 2 || a.Valid != 1 || a.Missing != 1 || b.Expected != 2 || b.Damaged != 1 || b.Unreadable != 1 {
		t.Fatalf("wrong categories: %+v", r.Groups)
	}
	if !reflect.DeepEqual(before, hashes(t, root)) {
		t.Fatal("inspection changed resources")
	}
	if r.CompletedAt.Before(r.StartedAt) {
		t.Fatal("invalid timestamp")
	}
}
func TestManifestBoundaries(t *testing.T) {
	for _, name := range []string{"../escape.png", "C:/escape.png", "/escape.png", `..\escape.png`, "a/../b.png", ""} {
		if _, err := manifestNames("wife", []byte(`["`+name+`"]`)); err == nil {
			t.Fatalf("accepted path %q", name)
		}
	}
	for _, data := range []string{`null`, `[]`, `{}`, `broken`} {
		if _, err := manifestNames("tarot", []byte(data)); err == nil {
			t.Fatalf("accepted manifest %s", data)
		}
	}
	r, err := Inspect(context.Background(), t.TempDir())
	if err != nil || r.Groups[0].Error == "" || r.Groups[1].Error == "" {
		t.Fatal("missing manifests reported as healthy")
	}
}
func TestSnapshotAndCancellation(t *testing.T) {
	m := New(fixture(t))
	if m.Read().Report != nil {
		t.Fatal("GET initiated a scan")
	}
	first, err := m.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = m.Check(ctx); err == nil {
		t.Fatal("cancel ignored")
	}
	if m.Read().Busy || m.Read().Report != first.Report {
		t.Fatal("cancellation discarded completed result")
	}
	m.busy = true
	if _, err = m.Check(context.Background()); err == nil {
		t.Fatal("overlapping scan accepted")
	}
	m.busy = false
}
func TestLinkedResourcesRejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	put(t, outside, "a.png", []byte("outside"))
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skip("symlink privilege unavailable")
	}
	if _, _, err := readLocal(root, "linked/a.png", 100); err == nil {
		t.Fatal("followed link outside root")
	}
}

func TestPixelLimit(t *testing.T) {
	// GIF logical screen dimensions are enough to reject excessive allocation.
	data := []byte{'G', 'I', 'F', '8', '9', 'a', 0x40, 0x1f, 0x40, 0x1f, 0, 0, 0}
	if !exceedsPixelLimit(data) {
		t.Fatal("pixel limit ignored")
	}
	root := fixture(t)
	put(t, root, "Tarot/a.png", data)
	r, err := Inspect(context.Background(), root)
	if err != nil || r.Groups[0].Unreadable != 1 || r.Groups[0].Damaged != 0 {
		t.Fatal("oversized image reported as damaged", err)
	}
}
func TestReadReasons(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{errPath, "路径不在允许范围"}, {errLink, "路径包含链接，未跟随读取"},
		{errRegular, "目标不是普通文件"}, {errSize, "文件超过读取大小上限（清单4 MiB，图片8 MiB）"},
		{os.ErrNotExist, "文件或上级目录不存在"}, {os.ErrPermission, "没有读取权限"},
		{&os.PathError{Op: "open", Path: "private/absolute/path", Err: os.ErrInvalid}, "读取失败，请检查文件权限或占用情况"},
	}
	for _, c := range cases {
		if got := readReason(c.err); got != c.want {
			t.Fatalf("%q != %q", got, c.want)
		}
	}
	root := fixture(t)
	r, err := Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range r.Groups {
		for _, f := range g.Files {
			if (f.Status == "正常") != (f.Reason == "") {
				t.Fatalf("unexpected reason: %+v", f)
			}
		}
	}
	put(t, root, "large", make([]byte, 101))
	if _, _, err := readLocal(root, "large", 100); err != errSize {
		t.Fatal("size not classified", err)
	}
	if _, _, err := readLocal(root, "Wife/wives/directory.png", 100); err != errRegular {
		t.Fatal("directory not classified", err)
	}
}
