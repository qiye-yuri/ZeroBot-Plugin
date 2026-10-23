package tarot

import (
	"bytes"
	"context"
	"encoding/json"
	zero "github.com/wdvxdr1123/ZeroBot"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Opt-in, read-only check of every installed card with network access cancelled.
func TestInstalledCardCacheOffline(t *testing.T) {
	folder := os.Getenv("TAROT_CACHE_TEST_DIR")
	if folder == "" {
		t.Skip("set TAROT_CACHE_TEST_DIR for installed-cache audit")
	}
	raw, err := os.ReadFile(filepath.Join(folder, "tarots.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cards cardSet
	if err = json.Unmarshal(raw, &cards); err != nil {
		t.Fatal(err)
	}
	if len(cards) != 78 {
		t.Fatalf("expected 78 cards, got %d", len(cards))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, card := range cards {
		for _, prefix := range []string{"", "Reverse/"} {
			name := prefix + card.ImgURL
			if _, err := loadCardImage(ctx, folder, name); err != nil {
				t.Errorf("offline cache %s: %v", name, err)
			}
		}
	}
}

func TestCardCacheAndDeadline(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow.png" {
			<-r.Context().Done()
			return
		}
		if r.URL.Path == "/bad.png" {
			_, _ = w.Write([]byte("not an image"))
			return
		}
		_, _ = w.Write(imageData.Bytes())
	}))
	defer server.Close()
	dir := t.TempDir()
	if _, err := fetchCardImage(context.Background(), dir, "ok.png", server.URL+"/", server.Client()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fetchCardImage(ctx, dir, "ok.png", server.URL+"/", server.Client()); err != nil {
		t.Fatal("cache must work offline", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := fetchCardImage(ctx, dir, "slow.png", server.URL+"/", server.Client()); err == nil {
		t.Fatal("deadline ignored")
	}
	if time.Since(start) > time.Second {
		t.Fatal("slow image blocked too long")
	}
	for _, name := range []string{"bad.png", "../escape.png"} {
		if _, err := fetchCardImage(context.Background(), dir, name, server.URL+"/", server.Client()); err == nil {
			t.Fatal("invalid image/path accepted", name)
		}
	}
}

func TestPrivateLimitsAreIndependent(t *testing.T) {
	a := &zero.Ctx{Event: &zero.Event{UserID: 98760001}}
	b := &zero.Ctx{Event: &zero.Event{UserID: 98760002}}
	for i := 0; i < 5; i++ {
		if !tarotLimit(a).Acquire() {
			t.Fatal("unexpected limit")
		}
	}
	if tarotLimit(a).Acquire() {
		t.Fatal("limit missing")
	}
	if !tarotLimit(b).Acquire() {
		t.Fatal("private users share limit")
	}
}
