package updatewatch

import (
	"encoding/json"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
	"path/filepath"
	"testing"
)

func TestNoticeShapeReceiptAndOutbox(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		code         int64
		data         string
		success      bool
	}{
		{"ok", "ok", 0, `{"message_id":123}`, true},
		{"negative-id", "ok", 0, `{"message_id":-123}`, true},
		{"failed-with-id", "failed", 0, `{"message_id":123}`, false},
		{"retcode-with-id", "ok", 1200, `{"message_id":123}`, false},
		{"no-receipt", "ok", 0, `{}`, false},
		{"zero-id", "ok", 0, `{"message_id":0}`, false},
		{"unknown", "", 0, `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			send := func(text string) bool {
				return sendNotice(func(action string, p zero.Params) zero.APIResponse {
					calls++
					if action != "send_private_msg" || p["user_id"] != int64(12345) {
						t.Fatal("wrong destination")
					}
					raw, e := json.Marshal(p["message"])
					if e != nil {
						t.Fatal(e)
					}
					var segments []struct {
						Type string
						Data map[string]string
					}
					if json.Unmarshal(raw, &segments) != nil || len(segments) != 1 || segments[0].Type != "text" || segments[0].Data["text"] != text {
						t.Fatalf("invalid message chain: %s", raw)
					}
					return zero.APIResponse{Status: tc.status, RetCode: tc.code, Data: gjson.Parse(tc.data)}
				}, 12345, text)
			}
			s := core.State{Seen: map[string]core.Event{}, Initialized: map[string]bool{}, Pending: []core.Notice{{Keys: []string{"fixture"}, Text: "[测试通知]"}}}
			path := filepath.Join(t.TempDir(), "state.json")
			if e := core.Save(path, s); e != nil {
				t.Fatal(e)
			}
			if e := core.Deliver(&s, path, send); e != nil {
				t.Fatal(e)
			}
			saved, e := core.LoadState(path)
			if e != nil {
				t.Fatal(e)
			}
			if calls != 1 || (len(saved.Pending) == 0) != tc.success || (len(saved.History) == 1) != tc.success {
				t.Fatal("lost or duplicated notification", calls, saved)
			}
		})
	}
}
