package modelstats

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestLabeledRequestsRetainOnlyAnonymousIdentity(t *testing.T) {
	c := New()
	client := c.Client(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return response(200, io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":0}}`))), nil
	})}, Agent, Labels{Model: "synthetic-model", Session: "private-session", Task: "private-task"})
	for i := 0; i < 2; i++ {
		r, err := client.Get("https://provider.invalid/secret-path?key=secret-key")
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
	}
	s := c.Read()
	a, b := s.Recent[0], s.Recent[1]
	if a.Provider != "provider.invalid" || a.Model != "synthetic-model" || a.Session == "" || a.Task == "" || a.Task != b.Task || a.Session != b.Session || a.Sequence != 1 || b.Sequence != 2 {
		t.Fatal("metadata correlation")
	}
	raw, _ := json.Marshal(s)
	for _, secret := range []string{"private-session", "private-task", "secret-path", "secret-key"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("metadata leaked")
		}
	}
}

func TestUsagePreservesResponseAndUnknown(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		known      bool
	}{
		{"deepseek", `{"choices":[{"secret":"not retained"}],"usage":{"prompt_tokens":100,"completion_tokens":3,"prompt_cache_hit_tokens":64,"prompt_cache_miss_tokens":36}}`, true},
		{"missing", `{"choices":[]}`, false},
		{"invalid", `{"usage":{"prompt_tokens":"100","completion_tokens":-1,"prompt_cache_hit_tokens":null,"prompt_cache_miss_tokens":1.5}}`, false},
		{"truncated", `{"usage":`, false},
		{"oversized", `{"padding":"` + strings.Repeat("x", 2<<20) + `","usage":{"prompt_tokens":100}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New()
			r, err := exchange(c, Agent, 200, io.NopCloser(strings.NewReader(tc.body)))
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(r.Body)
			r.Body.Close()
			if err != nil || string(b) != tc.body {
				t.Fatal("response changed", err)
			}
			s := c.Read()
			if len(s.Recent) != 1 {
				t.Fatal(s)
			}
			u := s.Recent[0].Usage
			if tc.known {
				if u.Input == nil || *u.Input != 100 || u.Hit == nil || *u.Hit != 64 || u.Miss == nil || *u.Miss != 36 || u.Output == nil || *u.Output != 3 {
					t.Fatal(u)
				}
				*u.Input = 999
				if *c.Read().Recent[0].Usage.Input != 100 {
					t.Fatal("aliased snapshot")
				}
			} else if u.Input != nil || u.Hit != nil || u.Miss != nil || u.Output != nil {
				t.Fatal("invented usage", u)
			}
			raw, _ := json.Marshal(s)
			if strings.Contains(string(raw), "not retained") {
				t.Fatal("payload leaked")
			}
		})
	}
}
func TestUsageBoundedAndFailuresUnknown(t *testing.T) {
	c := New()
	for i := 0; i < 260; i++ {
		r, _ := exchange(c, GPT, 503, io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":100}}`)))
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
	}
	s := c.Read()
	if len(s.Recent) != 256 || s.Recent[0].ID != 5 || s.Recent[255].ID != 260 || s.Recent[0].Usage.Input != nil {
		t.Fatal("retention or failure accounting")
	}
	if anonymous("private-id") == "private-id" || anonymous("private-id") != anonymous("private-id") || anonymous("") != "" {
		t.Fatal("identity handling")
	}
	u := parseUsage([]byte(`{"usage":{"prompt_tokens":0,"completion_tokens":0,"prompt_tokens_details":{"cached_tokens":0}}}`))
	if u.Input == nil || *u.Input != 0 || u.Hit == nil || *u.Hit != 0 || u.Miss != nil {
		t.Fatal("zero or unknown lost")
	}
}
