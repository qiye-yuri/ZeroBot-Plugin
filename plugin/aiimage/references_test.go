package aiimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/imagegen"
	sql "github.com/FloatTech/sqlite"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func referencePNG(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestReferenceImagesReachEditAPI(t *testing.T) {
	red := referencePNG(t, color.RGBA{R: 255, A: 255})
	blue := referencePNG(t, color.RGBA{B: 255, A: 255})
	var edits, generations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			if r.Header.Get("Authorization") != "" {
				t.Error("model key leaked to image server")
			}
			if r.URL.Path == "/source" {
				w.Write(red)
			} else {
				w.WriteHeader(404)
			}
			return
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-key" {
			t.Error("edit auth missing")
		}
		var body struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
			Images []struct {
				URL string `json:"image_url"`
			} `json:"images"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/v1/images/edits":
			edits.Add(1)
			if body.Model != "gpt-image-2.5-sunburst" || body.Prompt != "combine" || len(body.Images) != 2 {
				t.Errorf("edit payload incomplete")
				return
			}
			for i, want := range [][]byte{red, blue} {
				prefix, encoded, ok := strings.Cut(body.Images[i].URL, ",")
				if !ok || prefix != "data:image/png;base64" {
					t.Fatal("not inline PNG")
				}
				got, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil || !bytes.Equal(got, want) {
					t.Error("input pixels/bytes lost or order changed")
				}
			}
		case "/v1/images/generations":
			generations.Add(1)
			if len(body.Images) != 0 {
				t.Error("reference leaked into text generation")
			}
		default:
			t.Error("wrong endpoint", r.URL.Path)
		}
		io.WriteString(w, `{"data":[{"b64_json":"aW1hZ2U="}]}`)
	}))
	defer server.Close()
	refs, err := prepareReferences(context.Background(), server.Client(), []referenceSource{{URL: server.URL + "/source", File: "opaque.png"}, {File: "base64://" + base64.StdEncoding.EncodeToString(blue)}}, nil)
	if err != nil || len(refs) != 2 {
		t.Fatal(refs, err)
	}
	if refs[0].SHA256 != fmt.Sprintf("%x", sha256.Sum256(red)) {
		t.Fatal("wrong original bytes")
	}
	cfg := imageConfig{APIURL: server.URL + "/v1/images/generations", ModelName: "gpt-image-2.5-sunburst", APIKey: "synthetic-key"}
	if _, err = generateImageWithPreview(server.Client(), cfg, "openai", "combine", defaultSettings(), nil, refs...); err != nil {
		t.Fatal(err)
	}
	if _, err = generateImage(server.Client(), cfg, "openai", "text only"); err != nil {
		t.Fatal(err)
	}
	if edits.Load() != 1 || generations.Load() != 1 {
		t.Fatal(edits.Load(), generations.Load())
	}
}
func TestReferenceFailuresAndResolution(t *testing.T) {
	data := referencePNG(t, color.RGBA{G: 255, A: 255})
	var resolves int
	refs, err := prepareReferences(context.Background(), http.DefaultClient, []referenceSource{{File: "opaque.png"}}, func(file string) referenceSource {
		resolves++
		if file != "opaque.png" {
			t.Fatal(file)
		}
		return referenceSource{File: "base64://" + base64.StdEncoding.EncodeToString(data)}
	})
	if err != nil || resolves != 1 || len(refs) != 1 {
		t.Fatal(refs, err, resolves)
	}
	for _, source := range []string{"file:///private.png", "C:\\secret.png", "base64://!", "data:text/plain;base64,aW1hZ2U=", "base64://" + base64.StdEncoding.EncodeToString([]byte("not image"))} {
		if _, err = prepareReferences(context.Background(), http.DefaultClient, []referenceSource{{File: source}}, nil); err == nil {
			t.Fatal("invalid reference accepted", source)
		}
	}
	if _, err = prepareReferences(context.Background(), http.DefaultClient, make([]referenceSource, 17), nil); err == nil {
		t.Fatal("too many")
	}
	if _, err = editEndpoint("https://example.com/custom"); err == nil {
		t.Fatal("guessed custom endpoint")
	}
	if _, err = referenceBudget(imageConfig{APIURL: "https://example.com/v1/images/generations"}, "siliconflow", defaultSettings(), refs); err == nil {
		t.Fatal("legacy mode ignored reference")
	}
	if _, err = readImageStream(strings.NewReader("data: {\"type\":\"image_edit.completed\",\"b64_json\":\"aW1hZ2U=\"}\n\n"), 1, 0, nil); err != nil {
		t.Fatal(err)
	}
}
func TestReferenceBudgetAddedBeforeRequest(t *testing.T) {
	s := budgetFixture(t, "0.3", "0.1")
	view, err := s.admin(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	update := &core.Update{Revision: view.Revision, Mode: "openai", URL: "https://api.openai.com/v1/images/generations", Model: "gpt-image-2.5-sunburst", MonthlyBudgetUSD: "0.3"}
	update.Key.Action = "keep"
	if _, err = s.admin(update, func(*sql.Sqlite) error { return nil }); err != nil {
		t.Fatal(err)
	}
	data := referencePNG(t, color.RGBA{R: 255, A: 255})
	refs, err := prepareReferences(context.Background(), http.DefaultClient, []referenceSource{{File: "base64://" + base64.StdEncoding.EncodeToString(data)}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _, entry, err := s.reserve("cat", time.Now(), refs...)
	if err != nil || entry.Amount != 270939 {
		t.Fatal(entry, err)
	}
	if _, _, _, _, _, err = s.reserve("cat", time.Now(), refs...); err == nil {
		t.Fatal("reference input bypassed budget")
	}
	saved, err := s.admin(nil, nil)
	if err != nil || saved.Budget.ChargedUSD != "0.270939" {
		t.Fatal(saved, err)
	}
}

func TestOldFormPreservesReferenceAllowance(t *testing.T) {
	s := budgetFixture(t, "100", "0.1")
	v, err := s.admin(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	o := defaultOptions()
	o.ReferenceReserveUSD = "0.5"
	u := &core.Update{Revision: v.Revision, Mode: v.Mode, URL: v.URL, Model: v.Model, CustomCostUSD: v.CustomCostUSD, Options: &o}
	u.Key.Action = "keep"
	v, err = s.admin(u, func(*sql.Sqlite) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	u.Revision = v.Revision
	o.ReferenceReserveUSD = ""
	v, err = s.admin(u, func(*sql.Sqlite) error { return nil })
	if err != nil || v.Options.ReferenceReserveUSD != "0.5" {
		t.Fatal(v, err)
	}
}

func TestOneBotImageCacheFallback(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "123", "nt_qq", "nt_data", "Pic", "month", "Ori")
	os.MkdirAll(dir, 0700)
	data := referencePNG(t, color.RGBA{R: 255, A: 255})
	path := filepath.Join(dir, "image.png")
	os.WriteFile(path, data, 0600)
	src := oneBotReferenceSource("", path, "image.png", 123)
	refs, err := prepareReferences(context.Background(), http.DefaultClient, []referenceSource{{File: "image.png"}}, func(string) referenceSource { return src })
	if err != nil || len(refs) != 1 {
		t.Fatal(err)
	}
	if oneBotReferenceSource("", path, "image.png", 999).File != "" {
		t.Fatal("other bot cache accepted")
	}
	outside := filepath.Join(root, "image.png")
	os.WriteFile(outside, data, 0600)
	if oneBotReferenceSource("", outside, "image.png", 123).File != "" {
		t.Fatal("arbitrary file accepted")
	}
}

func TestOneBotUUIDCacheFallback(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "123", "nt_qq", "nt_data", "Pic", "month", "Ori")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	cache := "0123456789abcdef0123456789abcdef.png"
	file := "{01234567-89AB-CDEF-0123-456789ABCDEF}.png"
	path := filepath.Join(dir, cache)
	data := referencePNG(t, color.RGBA{G: 255, A: 255})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	refs, err := prepareReferences(context.Background(), http.DefaultClient, []referenceSource{{File: file}}, func(id string) referenceSource { return oneBotReferenceSource("", path, id, 123) })
	if err != nil || len(refs) != 1 || refs[0].SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) {
		t.Fatal("UUID cache not loaded intact", err)
	}
	for _, invalid := range []string{"{11234567-89AB-CDEF-0123-456789ABCDEF}.png", "{01234567-89AB-CDEF-0123-456789ABCDEF}.jpg", "../" + file, "{0123456789ABCDEF0123456789ABCDEF}.png"} {
		if oneBotReferenceSource("", path, invalid, 123).File != "" {
			t.Fatal("invalid name accepted", invalid)
		}
	}
	if oneBotReferenceSource("", path, file, 456).File != "" {
		t.Fatal("other bot cache accepted")
	}
}
