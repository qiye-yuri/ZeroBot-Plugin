package chat

import (
	"strings"
	"testing"
)

func TestHistoryMediaCompactionPreservesArchive(t *testing.T) {
	s, e := openAgentHistory(t.TempDir(), 456, 900000, historyConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	marker := strings.Repeat("ABCDEFGHIJKLMNOP", 4000)
	raw := `{"action":"send_group_msg","params":{"group_id":9007199254740993,"message":[{"type":"text","data":{"text":"keep me"}},{"type":"image","data":{"file":"base64://` + marker + `"}}]}}`
	v := mediaTestString(raw)
	if e = s.Append(10, "request", v); e != nil {
		t.Fatal(e)
	}
	body := historyBody(t, s, 10)
	if strings.Contains(body, marker) || !strings.Contains(body, "keep me") || !strings.Contains(body, "9007199254740993") || !strings.Contains(body, "历史媒体二进制已省略") {
		t.Fatal("model copy not compacted correctly")
	}
	var saved string
	if e = s.db.QueryRow("SELECT payload FROM history WHERE conversation=10").Scan(&saved); e != nil || !strings.Contains(saved, marker) {
		t.Fatal("archive changed", e)
	}
	if s.budget != 900000 {
		t.Fatal("budget changed")
	}
}
func TestHistoryMediaCompactionBoundaries(t *testing.T) {
	for _, raw := range []string{`not json base64://abc`, `{"type":"text","data":{"text":"base64://abc"}}`, `{"type":"image","data":{"file":"https://example.invalid/base64.png"}}`, `{"count":9007199254740993}`} {
		if compactHistoryMedia(raw) != raw {
			t.Fatal("unrelated data changed")
		}
	}
	for _, kind := range []string{"image", "record", "video"} {
		raw := `{"content":[{"type":"` + kind + `","data":{"file":"data:image/png;base64,QUJD","other":"keep"}}]}`
		out := compactHistoryMedia(raw)
		if strings.Contains(out, "QUJD") || !strings.Contains(out, "keep") || compactHistoryMedia(out) != out {
			t.Fatal("nested media or idempotence failed")
		}
	}
}

type mediaTestString string

func (s mediaTestString) String() string               { return string(s) }
func (s mediaTestString) MarshalJSON() ([]byte, error) { return []byte(s), nil }
