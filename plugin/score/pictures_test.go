package score

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPictureValidationAndFallback(t *testing.T) {
	for _, raw := range [][]byte{[]byte("<html>error</html>"), []byte("{\"error\":true}"), fallbackPicture(true)[:40]} {
		if _, err := normalizePicture(raw, true); err == nil {
			t.Fatal("invalid image accepted")
		}
	}
	if _, err := normalizePicture(fallbackPicture(false), true); err == nil {
		t.Fatal("small background accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "background.png")
	_ = os.WriteFile(path, []byte("invalid cache"), 0600)
	avatar, err := preparePictures(path, server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{avatar, mustRead(t, path)} {
		if _, format, err := image.Decode(bytes.NewReader(raw)); err != nil || format != "png" {
			t.Fatal(format, err)
		}
	}
}
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestWebPCacheNormalized(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(webPFixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "background.png")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(fallbackPicture(false)) }))
	defer server.Close()
	if _, err = preparePictures(path, server.URL, server.URL); err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(mustRead(t, path)))
	if err != nil || format != "png" || cfg.Width != 512 {
		t.Fatal(cfg, format, err)
	}
}

// Synthetic solid-color WebP, generated for this test.
const webPFixture = "UklGRi4AAABXRUJQVlA4TCIAAAAv/8F/AAdQ6dLWvP8BgUCyv/cMRfQ/4z//+c9//vOf//wf"

type offlineTransport struct{}

func (offlineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("offline fixture")
}
func TestAllSigninStylesOffline(t *testing.T) {
	old := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: offlineTransport{}}
	defer func() { http.DefaultClient = old }()
	for i, draw := range styles {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			dir := t.TempDir()
			a := &scdata{picfile: filepath.Join(dir, "background.png"), drawedfile: filepath.Join(dir, "signin.png"), uid: 12345678, nickname: "合成测试", score: 100, level: 3, rank: 0}
			img, err := draw(a)
			if err != nil || img == nil {
				t.Fatal(err)
			}
		})
	}
}
