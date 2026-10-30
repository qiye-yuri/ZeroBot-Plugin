package aiimage

import (
	"encoding/json"
	"fmt"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/imagegen"
	sql "github.com/FloatTech/sqlite"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Independent reference vectors captured from the official 2.5 calculator,
// not computed from the production formula in these tests.
func TestImage25OfficialCalculatorVectors(t *testing.T) {
	for _, tc := range []struct {
		quality, size string
		tokens        int64
	}{
		{"low", "1024x1024", 196}, {"low", "1536x1024", 158}, {"low", "1024x1536", 158},
		{"medium", "1024x1024", 439}, {"medium", "1536x1024", 343}, {"medium", "1024x1536", 343},
		{"high", "1024x1024", 1756}, {"high", "1536x1024", 1372}, {"high", "1024x1536", 1372},
	} {
		t.Run(tc.quality+"/"+tc.size, func(t *testing.T) {
			got, err := image25OutputTokens(tc.quality, tc.size)
			if err != nil || got != tc.tokens {
				t.Fatal(got, err)
			}
			for _, model := range []string{"gpt-image-2.5-flare", "gpt-image-2.5-sunburst", "gpt-image-2.5-flare-2026-09-08", "gpt-image-2.5-sunburst-2026-09-08"} {
				settings := defaultSettings()
				settings.Quality = tc.quality
				settings.Size = tc.size
				price, _, err := quoteImage(imageConfig{APIURL: "https://api.openai.com/v1/images/generations", ModelName: model}, "openai", "cat", settings)
				expected := (tc.tokens*30*120+99)/100 + 1027*5
				if err != nil || price != expected {
					t.Fatalf("%s: %d != %d (%v)", model, price, expected, err)
				}
			}
		})
	}
	for _, q := range []string{"ultra", "invalid", ""} {
		if _, err := image25OutputTokens(q, "1024x1024"); err == nil {
			t.Fatal("unpriced quality", q)
		}
	}
	if _, err := image25OutputTokens("medium", "4096x4096"); err == nil {
		t.Fatal("unpriced size")
	}
	if got, _ := image25OutputTokens("medium", "1024x1024"); got == 1756 {
		t.Fatal("used GPT Image 2 coefficients")
	}
}

type image25Transport func(*http.Request) (*http.Response, error)

func (f image25Transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestImage25OfficialAdmissionRequestAndPersistence(t *testing.T) {
	for _, model := range []string{"gpt-image-2.5-flare", "gpt-image-2.5-sunburst"} {
		t.Run(model, func(t *testing.T) {
			s := &storage{}
			if err := s.open(filepath.Join(t.TempDir(), "image.db")); err != nil {
				t.Fatal(err)
			}
			defer s.db.Close()
			initial, err := s.admin(nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			update := &core.Update{Revision: initial.Revision, Mode: "openai", URL: "https://api.openai.com/v1/images/generations", Model: model}
			update.Key.Action = "replace"
			update.Key.Value = "synthetic-key"
			saved, err := s.admin(update, func(*sql.Sqlite) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			if !saved.PricingReady || saved.Quality != "medium" || saved.MonthlyBudgetUSD != "100.000000" {
				t.Fatal("defaults/price", saved)
			}
			// A request with the official URL uses an in-memory transport: no network.
			c, mode, settings, prompt, entry, err := s.reserve("cat", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if entry.Amount != 20939 {
				t.Fatal("expected standard rates + existing margin", entry.Amount)
			}
			calls := 0
			client := &http.Client{Transport: image25Transport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != update.URL || r.Header.Get("Authorization") != "Bearer synthetic-key" {
					t.Fatal("endpoint/auth")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["model"] != model || body["quality"] != "medium" || body["size"] != "1024x1024" || body["n"] != float64(1) {
					t.Fatal(body)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"aW1hZ2U="}]}`)), Header: make(http.Header)}, nil
			})}
			images, err := generateImage(client, c, mode, prompt, settings)
			if err != nil || len(images) != 1 || calls != 1 {
				t.Fatal(images, err, calls)
			}
			if err = s.finish(entry, true); err != nil {
				t.Fatal(err)
			}
			// Set limit equal to the already consumed quote; the next request cannot run.
			saved, err = s.admin(nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			update.Revision = saved.Revision
			update.MonthlyBudgetUSD = dollars(entry.Amount)
			update.Key.Action = "keep"
			update.Key.Value = ""
			if _, err = s.admin(update, func(*sql.Sqlite) error { return nil }); err != nil {
				t.Fatal(err)
			}
			if _, _, _, _, _, err = s.reserve("cat", time.Now()); err == nil {
				t.Fatal("exhausted budget admitted")
			}
			other := &storage{}
			if err = other.open(s.path); err != nil {
				t.Fatal(err)
			}
			defer other.db.Close()
			if _, _, _, _, _, err = other.reserve("cat", time.Now()); err == nil {
				t.Fatal("restart reset budget")
			}
			if calls != 1 {
				t.Fatal(fmt.Sprintf("extra request: %d", calls))
			}
		})
	}
}
