package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	goba "github.com/fumiama/go-onebot-agent"
)

type credentialFixtureJSON string

func (v credentialFixtureJSON) String() string               { return string(v) }
func (v credentialFixtureJSON) MarshalJSON() ([]byte, error) { return []byte(v), nil }

func TestCredentialSettingHistory(t *testing.T) {
	for _, command := range []string{"设置AI聊天密钥", "设置AI聊天Agent密钥", "设置AI聊天识图密钥", "设置AI画图密钥", "设置b站cookie ", "设置b站cooki "} {
		t.Run(command, func(t *testing.T) {
			h, err := openAgentHistory(t.TempDir(), 456, 900000, historyConfig)
			if err != nil {
				t.Fatal(err)
			}
			defer h.Close()
			if err = h.Append(10, "event", historyEvent(10, command+"fixture-secret")); err != nil {
				t.Fatal(err)
			}
			var raw string
			if err = h.db.QueryRow("SELECT payload FROM history").Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(raw, "fixture-secret") {
				t.Fatal("credential command persisted")
			}
			var ev map[string]any
			if json.Unmarshal([]byte(raw), &ev) != nil || ev["message_id"] != float64(1) || ev["group_id"] != float64(10) {
				t.Fatal("event identity lost")
			}
			if strings.Contains(historyBody(t, h, 10), "fixture-secret") {
				t.Fatal("credential reached model")
			}
		})
	}
}

func TestCredentialSettingToolMessagesAndIdentity(t *testing.T) {
	h, err := openAgentHistory(t.TempDir(), 456, 900000, historyConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	// A command split across text segments, with a raw-message duplicate.
	request := json.RawMessage(`{"action":"send_group_msg","params":{"group_id":20,"message":[{"type":"text","data":{"text":"设置AI画"}},{"type":"text","data":{"text":"图密钥fixture-secret"}}],"raw_message":"设置AI画图密钥fixture-secret"}}`)
	if _, err = h.appendWithSequence(10, "request", credentialFixtureJSON(request), "credential-test"); err != nil {
		t.Fatal(err)
	}
	response := json.RawMessage(`{"data":{"message_id":9007199254740993,"message":"设置b站cookie fixture-cookie"},"status":"ok","retcode":0}`)
	var rsp goba.APIResponse
	if err = json.Unmarshal(response, &rsp); err != nil {
		t.Fatal(err)
	}
	if _, err = h.appendWithSequence(10, "response", &rsp, "credential-test"); err != nil {
		t.Fatal(err)
	}
	var run string
	if err = h.db.QueryRow(`SELECT run_id FROM test_message_ids WHERE conversation=20 AND message_id='9007199254740993'`).Scan(&run); err != nil || run != "credential-test" {
		t.Fatal("test reply identity lost", err)
	}
	var leaked int
	if err = h.db.QueryRow(`SELECT COUNT(*) FROM history WHERE payload LIKE '%fixture-%'`).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatal("tool payload leaked", err)
	}
	ordinary := `{"message_id":9007199254740993,"message":[{"type":"text","data":{"text":"普通聊天🙂"}}]}`
	if redactCredentialPayload(ordinary) != ordinary {
		t.Fatal("ordinary payload changed")
	}
	for _, summary := range []string{"曾设置AI画图密钥fixture-secret", "123 设置b站cookie fixture-cookie"} {
		if redactCredentialPayload(summary) != hiddenCredentialSetting {
			t.Fatal("plain summary command leaked")
		}
	}
	if strings.Contains(historyBody(t, h, 10), hiddenCredentialSetting) {
		t.Fatal("test data flowed into normal context")
	}
}

func TestLegacyCredentialSettingReadAndSummary(t *testing.T) {
	h := summaryFixture(t)
	raw, _ := json.Marshal(historyEvent(10, "设置AI画图密钥legacy-fixture-secret"))
	// Simulate an existing archive from a version lacking image-key redaction.
	if _, err := h.db.Exec(`UPDATE history SET payload=? WHERE seq=1`, string(raw)); err != nil {
		t.Fatal(err)
	}
	items, _, err := h.recent(10)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if strings.Contains(item.payload, "legacy-fixture-secret") {
			t.Fatal("legacy read leaked")
		}
	}
	page, err := readArchive(context.Background(), h.root, 456, ArchiveQuery{Conversation: 10, Sequence: 1}, historyConfig)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "legacy-fixture-secret") || !strings.Contains(string(encoded), hiddenCredentialSetting) {
		t.Fatal("archive redaction missing")
	}
	seen := make(chan SummaryRequest, 1)
	h.scheduleSummary(10, func(r SummaryRequest) (string, error) { seen <- r; return "合成摘要", nil })
	select {
	case input := <-seen:
		if strings.Contains(input.Records, "legacy-fixture-secret") || !strings.Contains(input.Records, hiddenCredentialSetting) {
			t.Fatal("summary input leaked")
		}
	case <-time.After(time.Second):
		t.Fatal("summary not scheduled")
	}
	waitSummary(t, h)
	var stored string
	if err = h.db.QueryRow(`SELECT payload FROM history WHERE seq=1`).Scan(&stored); err != nil || stored != string(raw) {
		t.Fatal("legacy archive rewritten", err)
	}
	// A recent legacy event must also be masked in the final model request.
	if _, err = h.db.Exec(`UPDATE history SET payload=? WHERE seq=24`, string(raw)); err != nil {
		t.Fatal(err)
	}
	if body := historyBody(t, h, 10); strings.Contains(body, "legacy-fixture-secret") || !strings.Contains(body, hiddenCredentialSetting) {
		t.Fatal("legacy model replay leaked")
	}
}
