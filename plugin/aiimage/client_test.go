package aiimage

import (
	"encoding/json"
	sql "github.com/FloatTech/sqlite"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateProtocols(t *testing.T) {
	for _, mode := range []string{"openai", "siliconflow"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer synthetic-key" {
					t.Error("request method or auth")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != "synthetic-model" || body["prompt"] != "synthetic prompt" {
					t.Error("model/prompt")
				}
				if mode == "openai" {
					if body["size"] != "1024x1024" || body["n"] != float64(1) || body["quality"] != "medium" || body["image_size"] != nil || body["batch_size"] != nil {
						t.Errorf("unexpected payload: %v", body)
					}
					w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`))
				} else {
					if body["batch_size"] != float64(1) || body["image_size"] != "1024x1024" || body["n"] != nil {
						t.Error("legacy payload")
					}
					w.Write([]byte(`{"images":[{"url":"https://example.com/image.png"}]}`))
				}
			}))
			defer server.Close()
			got, err := generateImage(server.Client(), imageConfig{APIURL: server.URL, APIKey: "synthetic-key", ModelName: "synthetic-model"}, mode, "synthetic prompt")
			if err != nil || len(got) != 1 {
				t.Fatalf("%v %v", got, err)
			}
			if mode == "openai" && got[0] != "base64://aW1hZ2U=" {
				t.Fatal(got)
			}
		})
	}
}

func TestLocalAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"local-url", `{"data":[{"url":"http://127.0.0.1/image.png"}]}`, 200, true},
		{"empty", `{"data":[]}`, 200, false},
		{"invalid-base64", `{"data":[{"b64_json":"!"}]}`, 200, false},
		{"bad-json", `<html>bad gateway</html>`, 200, false},
		{"error", `{"error":{"message":"synthetic-key"}}`, 200, false},
		{"unauthorized", `synthetic-key`, 401, false},
		{"rate-limit", `{}`, 429, false},
		{"file-url", `{"data":[{"url":"file:///private"}]}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("local auth must be absent")
				}
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, err := generateImage(server.Client(), imageConfig{APIURL: server.URL}, "openai", "test")
			if (err == nil) != tc.ok {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "synthetic-key") {
				t.Fatal("secret leak")
			}
		})
	}
}

func TestIndependentConfigCompatibility(t *testing.T) {
	s := &storage{db: sql.New(filepath.Join(t.TempDir(), "aiimage.db"))}
	if err := s.db.Open(time.Hour); err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	// Simulate an existing installation with the original schema and credentials.
	if err := s.db.Create("config", &imageConfig{}); err != nil {
		t.Fatal(err)
	}
	if err := s.setConfig("synthetic-secret", "https://example.com/generate", "old-model"); err != nil {
		t.Fatal(err)
	}
	before := s.getConfig()
	if err := s.db.Create("protocol", &protocolConfig{}); err != nil {
		t.Fatal(err)
	}
	if s.getProtocol() != "siliconflow" {
		t.Fatal("legacy default changed")
	}
	if err := s.setProtocol("openai"); err != nil {
		t.Fatal(err)
	}
	if s.getProtocol() != "openai" || s.getConfig() != before {
		t.Fatal("credentials changed")
	}
	if err := s.setProtocol("invalid"); err == nil {
		t.Fatal("accepted invalid mode")
	}
	if strings.Contains(s.PrintConfig(), "synthetic-secret") {
		t.Fatal("config leaks key")
	}
	if err := s.setConfig("", before.APIURL, before.ModelName); err != nil {
		t.Fatal(err)
	}
	if s.getConfig().APIKey != "" {
		t.Fatal("clear key failed")
	}
}
