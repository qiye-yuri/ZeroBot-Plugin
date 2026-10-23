package wife

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeNamesAndValidation(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"角色甲", ".upload-pending"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	names, err := nativeNames(dir)
	if err != nil || len(names) != 1 || names[0] != "角色甲" {
		t.Fatalf("names=%v err=%v", names, err)
	}
	for _, name := range []string{"", ".", "..", "../outside", `a\b`, "C:x", "CON", "LPT1.png", "x.", "x\n"} {
		if validNativeName(name) {
			t.Errorf("accepted %q", name)
		}
	}
	if !validNativeName("角色甲.png") {
		t.Fatal("ordinary name rejected")
	}
	if nativeFolder(101) == nativeFolder(102) {
		t.Fatal("group isolation lost")
	}
}

func TestNativeDownloadRejectsFailures(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Write(buf.Bytes())
		case "/bad":
			w.Write([]byte("not an image"))
		default:
			w.WriteHeader(502)
		}
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "photo")
	if err := downloadNative(srv.URL+"/ok", path); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"/bad", "/error"} {
		if err := downloadNative(srv.URL+endpoint, path); err == nil {
			t.Fatalf("accepted %s", endpoint)
		}
		raw, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(raw, buf.Bytes()) {
			t.Fatal("failed download damaged existing image")
		}
	}
}
