package aiimage

import (
	"encoding/json"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/imagegen"
	sql "github.com/FloatTech/sqlite"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtendedOptionsValidationAndBudget(t *testing.T) {
	cfg := imageConfig{APIURL: "https://api.openai.com/v1/images/generations", ModelName: "gpt-image-2.5-sunburst"}
	v := defaultSettings()
	clean, v, err := parseImageOptions("--size=4k --quality=max --background=transparent --format=webp --compression=80 --n=2 --stream=true --partial-images=3 海边", v)
	if err != nil || clean != "海边" || v.Size != "3840x2160" {
		t.Fatal(clean, v, err)
	}
	if err = validateOptions(cfg, "openai", v); err != nil {
		t.Fatal(err)
	}
	o, _ := optionsFor(v)
	if o.Count != 2 || o.Format != "webp" || o.PartialImages != 3 {
		t.Fatal(o)
	}
	// Official formula: grid96, rounded short grid54, 4K output 13342 tokens.
	tokens, err := image25OutputTokens("max", "3840x2160")
	if err != nil || tokens != 13342 {
		t.Fatal(tokens, err)
	}
	price, _, err := quoteImage(cfg, "openai", clean, v)
	expected := ((tokens+300)*30*120/100 + (int64(len(clean))+1024)*5) * 2
	if err != nil || price != expected {
		t.Fatal(price, expected, err)
	}
	auto := v
	auto.Size = "auto"
	auto.Quality = "auto"
	maxPrice, _, err := quoteImage(cfg, "openai", clean, auto)
	if err != nil || maxPrice < price {
		t.Fatal(maxPrice, price, err)
	}
	for _, model := range []string{"gpt-image-1-mini", "gpt-image-1.5", "gpt-image-2"} {
		c := cfg
		c.ModelName = model
		if err = validateOptions(c, "openai", v); err == nil {
			t.Fatal("unsupported max", model)
		}
	}
	o.Format = "jpeg"
	putOptions(&v, o)
	if err = validateOptions(cfg, "openai", v); err == nil {
		t.Fatal("transparent JPEG")
	}
	for _, size := range []string{"4096x4096", "1920x1080", "3840x3840", "512x512", "3840x1024", "0x1024", "999999999x1024"} {
		if validSize(size) {
			t.Fatal(size)
		}
	}
	for _, size := range []string{"2048x2048", "2048x1152", "3840x2160", "2160x3840", "2880x2880", "1920x1088", "auto"} {
		if !validSize(size) {
			t.Fatal(size)
		}
	}
	if _, _, err = parseImageOptions("--proxy=http://evil.example 猫", defaultSettings()); err == nil {
		t.Fatal("chat proxy override allowed")
	}
	if _, _, err = parseImageOptions("--n=two 猫", defaultSettings()); err == nil {
		t.Fatal("non-number")
	}
}
func TestFullRequestPayloadAndStream(t *testing.T) {
	cfg := imageConfig{ModelName: "gpt-image-2.5-flare"}
	v := defaultSettings()
	v.Size = "3840x2160"
	v.Quality = "xhigh"
	o := defaultOptions()
	o.Background = "transparent"
	o.Format = "webp"
	o.Compression = 0
	o.Count = 2
	o.Moderation = "low"
	o.Stream = true
	o.PartialImages = 1
	o.SafetyIdentifier = "synthetic-hash"
	putOptions(&v, o)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		want := map[string]any{"model": cfg.ModelName, "prompt": "test", "size": "3840x2160", "quality": "xhigh", "background": "transparent", "output_format": "webp", "output_compression": float64(0), "n": float64(2), "moderation": "low", "stream": true, "partial_images": float64(1), "safety_identifier": "synthetic-hash"}
		a, _ := json.Marshal(body)
		b, _ := json.Marshal(want)
		if string(a) != string(b) {
			t.Errorf("payload=%s", a)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: image_generation.partial_image\ndata: {\"b64_json\":\"aW1hZ2U=\"}\n\n")
		io.WriteString(w, "data: {\"type\":\"image_generation.completed\",\"image_index\":0,\"b64_json\":\"aW1hZ2U=\"}\n\n")
		io.WriteString(w, "data: {\"type\":\"image_generation.completed\",\"image_index\":1,\"b64_json\":\"aW1hZ2U=\"}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	cfg.APIURL = srv.URL
	previews := 0
	images, err := generateImageWithPreview(srv.Client(), cfg, "openai", "test", v, func(img string, index int) {
		previews++
		if !strings.HasPrefix(img, "base64://") || index != 1 {
			t.Error(img, index)
		}
	})
	if err != nil || len(images) != 2 || previews != 1 {
		t.Fatal(images, previews, err)
	}
	for _, body := range []string{"data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"aW1hZ2U=\"}\n\n", "data: {\"type\":\"error\"}\n\n", "data: invalid\n\n"} {
		if _, err = readImageStream(strings.NewReader(body), 1, 1, nil); err == nil {
			t.Fatal("incomplete stream accepted")
		}
	}
}
func TestDedicatedImageProxy(t *testing.T) {
	seen := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = true
		if r.URL.Host != "synthetic.invalid" {
			t.Error(r.URL.String())
		}
		io.WriteString(w, `{"data":[{"b64_json":"aW1hZ2U="}]}`)
	}))
	defer srv.Close()
	v := defaultSettings()
	o := defaultOptions()
	o.ProxyURL = srv.URL
	putOptions(&v, o)
	_, err := generateImage(&http.Client{}, imageConfig{APIURL: "http://synthetic.invalid/v1/images/generations", ModelName: "synthetic"}, "openai", "test", v)
	if err != nil || !seen {
		t.Fatal(err, seen)
	}
}
func TestExtendedSettingsMigrationPreservesUserConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old := sql.New(path)
	if err := old.Open(time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec("CREATE TABLE render_settings (id INTEGER PRIMARY KEY,quality TEXT,size TEXT,budget INTEGER,customCost TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec("INSERT INTO render_settings VALUES(1,'high','1024x1024',100000000,'')"); err != nil {
		t.Fatal(err)
	}
	old.Close()
	s := &storage{}
	if err := s.open(path); err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	v, err := s.admin(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Quality != "high" || v.MonthlyBudgetUSD != "100.000000" || v.Options.Count != 1 || v.Options.Format != "png" || v.Options.Compression != 100 {
		t.Fatal(v)
	}
	update := &core.Update{Revision: v.Revision, Mode: "openai", URL: "http://localhost:8000/generate", Model: "local", Quality: "max", Size: "4k", CustomCostUSD: "0"}
	update.Key.Action = "keep"
	o := defaultOptions()
	o.Background = "transparent"
	o.Format = "webp"
	o.Compression = 0
	o.Count = 3
	update.Options = &o
	saved, err := s.admin(update, func(*sql.Sqlite) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if saved.Size != "3840x2160" || saved.Options.Compression != 0 || saved.Options.Count != 3 {
		t.Fatal(saved)
	}
	update.Revision = saved.Revision
	update.Options = nil
	update.Model = "other"
	again, err := s.admin(update, func(*sql.Sqlite) error { return nil })
	if err != nil || again.Options != saved.Options {
		t.Fatal(again, err)
	}
}

func TestImageCalculatorCustomRoundingBoundaries(t *testing.T) {
	for _, tc := range []struct {
		quality, size string
		want          int64
	}{{"high", "944x1536", 1201}, {"high", "1536x944", 1201}, {"xhigh", "784x2048", 1443}, {"high", "1888x3072", 2715}} {
		got, err := image25OutputTokens(tc.quality, tc.size)
		if err != nil || got != tc.want {
			t.Fatal(tc, got, err)
		}
	}
}
