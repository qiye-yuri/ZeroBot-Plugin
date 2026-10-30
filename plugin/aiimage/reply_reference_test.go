package aiimage

import (
	"encoding/json"
	"errors"
	"github.com/tidwall/gjson"
	"github.com/wdvxdr1123/ZeroBot/message"
	"testing"
)

func TestReplyImageScopeAndCommand(t *testing.T) {
	msg := message.Message{message.Reply(123), message.At(99), message.Text(" AI画图 --size=2k 修改背景")}
	if arg, ok := imageCommand(msg); !ok || arg != "--size=2k 修改背景" {
		t.Fatal(arg, ok)
	}
	base := map[string]any{"message_id": 123, "message_type": "private", "sender": map[string]any{"user_id": 10}, "user_id": 10, "time": 200, "message": []any{map[string]any{"type": "image", "data": map[string]string{"url": "https://example.com/image.png"}}}}
	allow := func(string) (bool, error) { return true, nil }
	for _, tc := range []struct {
		name   string
		group  int64
		change func(map[string]any)
		ok     bool
	}{
		{"incoming", 0, func(m map[string]any) {}, true},
		{"CQ string", 0, func(m map[string]any) { m["message"] = "[CQ:image,file=test.png,url=https://example.com/image.png]" }, true},
		{"bot reply", 0, func(m map[string]any) { m["sender"] = map[string]any{"user_id": 99}; m["target_id"] = 10 }, true},
		{"wrong private", 0, func(m map[string]any) { m["sender"] = map[string]any{"user_id": 11} }, false},
		{"unproven outgoing", 0, func(m map[string]any) { m["sender"] = map[string]any{"user_id": 99}; m["user_id"] = 99 }, false},
		{"same group", 20, func(m map[string]any) { m["group_id"] = 20; m["message_type"] = "group" }, true},
		{"other group", 20, func(m map[string]any) { m["group_id"] = 21 }, false},
		{"old test", 0, func(m map[string]any) { m["time"] = 50 }, false},
		{"wrong id", 0, func(m map[string]any) { m["message_id"] = 124 }, false},
		{"no image", 0, func(m map[string]any) { m["message"] = []any{} }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := map[string]any{}
			for k, v := range base {
				m[k] = v
			}
			tc.change(m)
			b, _ := json.Marshal(m)
			src, err := referenceSources(msg, 99, 10, tc.group, allow, func(int64) (gjson.Result, error) { return gjson.ParseBytes(b), nil }, 100)
			if (err == nil) != tc.ok || tc.ok && len(src) != 1 {
				t.Fatal(src, err)
			}
		})
	}
	called := false
	fetch := func(int64) (gjson.Result, error) { called = true; return gjson.Result{}, errors.New("missing") }
	if _, err := referenceSources(msg, 99, 10, 0, func(string) (bool, error) { return false, nil }, fetch, 0); err == nil || called {
		t.Fatal("denied reference was fetched")
	}
	direct := append(msg, message.Image("https://example.com/direct.png"))
	b, _ := json.Marshal(base)
	src, err := referenceSources(direct, 99, 10, 0, allow, func(int64) (gjson.Result, error) { return gjson.ParseBytes(b), nil }, 0)
	if err != nil || len(src) != 2 || called {
		t.Fatal("direct and quoted images combined", err)
	}
	if _, err = referenceSources(msg, 99, 10, 0, allow, fetch, 0); err == nil {
		t.Fatal("failed get_msg ignored")
	}
}
