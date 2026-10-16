package goba

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/fumiama/deepinfra"
	"github.com/fumiama/deepinfra/model"
)

func TestNormalizeImageDescription(t *testing.T) {
	for _, bad := range []string{"", " \n\t\u2003", string([]byte{0xff}), "binary\x00text"} {
		if d, ok := normalizeImageDescription(bad); ok || d != (imageDescription{}) {
			t.Fatal("invalid description accepted", d)
		}
	}
	for _, tc := range []struct {
		in, want  string
		truncated bool
	}{{"  图片 🙂 \n", "图片 🙂", false}, {strings.Repeat("a", 4000), strings.Repeat("a", 4000), false}, {strings.Repeat("🙂", 1000), strings.Repeat("🙂", 1000), false}, {strings.Repeat("字", 1335), strings.Repeat("字", 1333), true}, {strings.Repeat("a", 3999) + "🙂", strings.Repeat("a", 3999), true}} {
		d, ok := normalizeImageDescription(tc.in)
		if !ok || d.Text != tc.want || d.Truncated != tc.truncated || !utf8.ValidString(d.Text) || len(d.Text) > agentImageDescriptionBytes {
			t.Fatal("incorrect normalization", len(d.Text), d.Truncated)
		}
		data := map[string]string{"__agent_desc_truncated__": "stale"}
		d.apply(data)
		if data["__agent_desc__"] != tc.want || (data["__agent_desc_truncated__"] == "true") != tc.truncated {
			t.Fatal("annotation mismatch")
		}
		if !tc.truncated {
			if _, ok := data["__agent_desc_truncated__"]; ok {
				t.Fatal("stale annotation kept")
			}
		}
	}
}

func TestVisionDescriptionCacheAndExistingAnnotations(t *testing.T) {
	for _, kind := range []string{"long", "blank", "prefilled", "multipart", "missing", "unknown", "mixed", "genai-panic"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				text := "  useful description  "
				if kind == "long" {
					text = strings.Repeat("字", 1335) + "UNSEEN_TAIL"
				}
				if kind == "blank" && n == 1 {
					text = " \n\t "
				}
				if kind == "missing" && n == 1 {
					fmt.Fprint(w, `{}`)
					return
				}
				if kind == "genai-panic" {
					if n == 1 {
						fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{}]}}]}`)
					} else {
						fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"useful "},{"text":"description"}]}}]}`)
					}
					return
				}
				var content any = text
				if kind == "multipart" || (kind == "long") {
					cut := len(text) / 2
					for !utf8.ValidString(text[:cut]) {
						cut--
					}
					content = []any{map[string]any{"type": "text", "text": text[:cut]}, map[string]any{"type": "text", "text": text[cut:]}}
				}
				if (kind == "unknown" || kind == "mixed") && n == 1 {
					partType := "private-unknown-type"
					if kind == "mixed" {
						partType = "image_url"
					}
					content = []any{map[string]any{"type": "text", "text": "MUST_NOT_CACHE"}, map[string]any{"type": partType, "image_url": map[string]any{"url": "https://invalid.example/image"}}}
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}}}})
			}))
			defer server.Close()
			ag := NewAgent(&Config{}, 16, 8, time.Hour, "", nil, true, true)
			api := deepinfra.NewAPI(server.URL, "")
			api.SetHTTPClient(server.Client())
			ag.SetViewImageAPI(api, model.NewOpenAI("vision", "", 0, 1, 128))
			if kind == "genai-panic" {
				ag.SetViewImageAPI(api, model.NewGenAI("vision", 0, 1, 128))
			}
			source := "base64://" + base64.StdEncoding.EncodeToString(imageInputPNG(t))
			apply := func(data map[string]string) map[string]string {
				raw, _ := json.Marshal([]any{map[string]any{"type": "image", "data": data}})
				ev := &Event{Message: raw}
				var item fmt.Stringer = ev
				ag.historyPre(&item)
				var rows []struct {
					Data map[string]string `json:"data"`
				}
				if err := json.Unmarshal(ev.Message, &rows); err != nil {
					t.Fatal(err)
				}
				return rows[0].Data
			}
			if kind == "prefilled" {
				d := apply(map[string]string{"file": "file:///not-read.png", "__agent_desc__": strings.Repeat("字", 1335)})
				if len(d["__agent_desc__"]) != 3999 || d["__agent_desc_truncated__"] != "true" || calls.Load() != 0 {
					t.Fatal("existing description bypassed bound")
				}
				d = apply(map[string]string{"file": "file:///not-read.png", "__agent_desc__": "short", "__agent_desc_truncated__": "true"})
				if d["__agent_desc_truncated__"] != "true" {
					t.Fatal("lost previous truncation")
				}
				d = apply(map[string]string{"file": "file:///not-read.png", "__agent_desc__": " ", "__agent_desc_truncated__": "true"})
				if _, ok := d["__agent_desc__"]; ok {
					t.Fatal("blank existing description kept")
				}
				if _, ok := d["__agent_desc_truncated__"]; ok {
					t.Fatal("stale flag kept")
				}
				return
			}
			first := apply(map[string]string{"file": source})
			if kind == "blank" || kind == "missing" || kind == "unknown" || kind == "mixed" || kind == "genai-panic" {
				if _, ok := first["__agent_desc__"]; ok {
					t.Fatal("blank result attached")
				}
			}
			second := apply(map[string]string{"file": source})
			if kind == "long" {
				if calls.Load() != 1 || first["__agent_desc__"] != second["__agent_desc__"] || len(second["__agent_desc__"]) != 3999 || second["__agent_desc_truncated__"] != "true" || strings.Contains(second["__agent_desc__"], "UNSEEN_TAIL") {
					t.Fatal("truncated cache incorrect")
				}
			} else if kind == "multipart" {
				if calls.Load() != 1 || first["__agent_desc__"] != "useful description" || second["__agent_desc__"] != "useful description" {
					t.Fatal("multipart description or cache incorrect")
				}
			} else {
				if calls.Load() != 2 || second["__agent_desc__"] != "useful description" {
					t.Fatal("blank result blocked later recovery")
				}
				apply(map[string]string{"file": source})
				if calls.Load() != 2 {
					t.Fatal("valid recovery not cached")
				}
			}
		})
	}
}
