package modelstats

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestUsageIntegerAndPartialFieldBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      *int64
	}{
		{"zero", "0", int64ptr(0)},
		{"max", "9223372036854775807", int64ptr(math.MaxInt64)},
		{"overflow", "9223372036854775808", nil},
		{"negative", "-1", nil},
		{"fraction", "0.5", nil},
		{"string", `"42"`, nil},
		{"boolean", "true", nil},
		{"null", "null", nil},
		{"missing", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := token(json.RawMessage(tc.raw))
			if (got == nil) != (tc.want == nil) || got != nil && *got != *tc.want {
				t.Fatalf("token %q: %v", tc.raw, got)
			}
		})
	}
	u := parseUsage([]byte(`{"usage":{"input_tokens":42,"output_tokens":7,"prompt_cache_hit_tokens":-1,"prompt_cache_miss_tokens":null}}`))
	if u.Input == nil || *u.Input != 42 || u.Output == nil || *u.Output != 7 || u.Hit != nil || u.Miss != nil {
		t.Fatalf("partial usage lost or fabricated: %+v", u)
	}
	u = parseUsage([]byte(`{"usage":{"prompt_tokens":100,"completion_tokens":2,"prompt_cache_hit_tokens":0,"prompt_tokens_details":{"cached_tokens":80}}}`))
	if u.Hit == nil || *u.Hit != 0 || u.Miss != nil {
		t.Fatalf("explicit zero overridden or miss inferred: %+v", u)
	}
}

func int64ptr(n int64) *int64 { return &n }

func TestUsageCaptureChunkedExactLimitAndDiscard(t *testing.T) {
	const limit = 2 << 20
	prefix, suffix := `{"padding":"`, `","usage":{"prompt_tokens":42}}`
	body := prefix + strings.Repeat("x", limit-len(prefix)-len(suffix)) + suffix
	for _, oversized := range []bool{false, true} {
		capture := &usageCapture{}
		data := body
		if oversized {
			data += " "
		}
		for offset := 0; offset < len(data); offset += 137 {
			end := offset + 137
			if end > len(data) {
				end = len(data)
			}
			capture.add([]byte(data[offset:end]))
		}
		retained := capture.finish()
		u := parseUsage(retained)
		if oversized {
			if len(retained) != 0 || u.Input != nil {
				t.Fatal("oversized response retained")
			}
		} else if string(retained) != body || u.Input == nil || *u.Input != 42 {
			t.Fatal("chunked response at limit lost")
		}
		if capture.data != nil {
			t.Fatal("completed capture retained body")
		}
		capture.add([]byte(`{"usage":{"prompt_tokens":999}}`))
		if len(capture.finish()) != 0 {
			t.Fatal("late reads reopened finished capture")
		}
	}
}

func TestUsageSnapshotsDoNotAliasAnyTokenField(t *testing.T) {
	u := Usage{Input: int64ptr(100), Output: int64ptr(7), Hit: int64ptr(64), Miss: int64ptr(36)}
	c := New()
	c.recent = []Request{{Usage: u}}
	s := c.Read()
	*s.Recent[0].Usage.Input = 1
	*s.Recent[0].Usage.Output = 1
	*s.Recent[0].Usage.Hit = 1
	*s.Recent[0].Usage.Miss = 1
	got := c.Read().Recent[0].Usage
	if *got.Input != 100 || *got.Output != 7 || *got.Hit != 64 || *got.Miss != 36 {
		t.Fatal("snapshot mutated collector tokens")
	}
}
