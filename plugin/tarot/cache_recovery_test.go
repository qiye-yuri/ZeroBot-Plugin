package tarot

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCardCacheRecoveryAndWriteFailure(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	picture := encoded.Bytes()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/unavailable.png":
			w.WriteHeader(http.StatusBadGateway)
		case "/oversize.png":
			_, _ = w.Write(append(append([]byte(nil), picture...), make([]byte, 8<<20)...))
		default:
			_, _ = w.Write(picture)
		}
	}))
	defer server.Close()
	fetch := func(folder, name string) ([]byte, error) {
		return fetchCardImage(context.Background(), folder, name, server.URL+"/", server.Client())
	}
	t.Run("corrupt-cache-repaired-and-offline-readable", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "card.png"), []byte("interrupted download"), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := fetch(dir, "card.png")
		if err != nil || !bytes.Equal(data, picture) {
			t.Fatal("corrupt cache not recovered", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		data, err = fetchCardImage(ctx, dir, "card.png", server.URL+"/", server.Client())
		if err != nil || !bytes.Equal(data, picture) {
			t.Fatal("repaired cache not saved", err)
		}
		files, err := filepath.Glob(filepath.Join(dir, "card-*.tmp"))
		if err != nil || len(files) != 0 {
			t.Fatal("temporary files leaked", files, err)
		}
	})
	for _, name := range []string{"unavailable.png", "oversize.png"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			data, err := fetch(dir, name)
			if err == nil || len(data) != 0 {
				t.Fatal("invalid response accepted")
			}
			if _, err = os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
				t.Fatal("failed response cached", err)
			}
		})
	}
	t.Run("cache-write-failure-preserves-valid-response", func(t *testing.T) {
		blocked := filepath.Join(t.TempDir(), "file-not-directory")
		if err := os.WriteFile(blocked, []byte("unchanged"), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := fetch(blocked, "card.png")
		if err != nil || !bytes.Equal(data, picture) {
			t.Fatal("cache write failure discarded image", err)
		}
		contents, err := os.ReadFile(blocked)
		if err != nil || string(contents) != "unchanged" {
			t.Fatal("existing file changed", err)
		}
	})
}
