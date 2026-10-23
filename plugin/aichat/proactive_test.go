package aichat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FloatTech/zbputils/chat"
	"github.com/FloatTech/zbputils/ctxext"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

func TestProactiveIntervalFollowupIsolationAndRetry(t *testing.T) {
	p := proactiveTracker{groups: map[proactiveKey]*proactiveState{}}
	now := time.Unix(10000, 0)
	key := proactiveKey{1, 101}
	plan := func(at time.Time) (bool, []string) { return p.plan(key, at, 10*time.Minute, 2*time.Minute) }
	if due, turns := plan(now); due || len(turns) > 0 {
		t.Fatal("first new message must start timer")
	}
	if due, _ := plan(now.Add(10*time.Minute - time.Nanosecond)); due {
		t.Fatal("too early")
	}
	at := now.Add(10 * time.Minute)
	if due, _ := plan(at); !due {
		t.Fatal("due message not admitted")
	}
	if due, turns := plan(at.Add(time.Second)); due || len(turns) > 0 {
		t.Fatal("failed request must not flood")
	}
	p.delivered(key, at, "聊猫", "猫很可爱")
	if due, turns := plan(at.Add(time.Second)); due || len(turns) != 2 {
		t.Fatal("immediate related follow-up not offered")
	}
	if _, turns := plan(at.Add(2 * time.Second)); len(turns) > 0 {
		t.Fatal("rejected checks need throttle")
	}
	if _, turns := plan(at.Add(6 * time.Second)); len(turns) != 2 {
		t.Fatal("follow-up retry missing")
	}
	if due, turns := plan(at.Add(2 * time.Minute)); due || len(turns) > 0 {
		t.Fatal("expired thread followed")
	}
	for _, k := range []proactiveKey{{2, 101}, {1, 102}} {
		if due, turns := p.plan(k, at, time.Minute, time.Minute); due || len(turns) > 0 {
			t.Fatal("cross-account/group state leak")
		}
	}
	p.delivered(key, at.Add(3*time.Minute), "猫吃什么", "猫粮")
	if due, _ := plan(at.Add(10 * time.Minute)); due {
		t.Fatal("recent reply should renew idle interval")
	}
}

func TestProactiveSettingsDoNotOverlapLegacyFlags(t *testing.T) {
	s := ctxext.Storage(0).Set(100, chat.BitmapRate).Set(81, chat.BitmapTemp).Set(1, chat.BitmapNagt)
	s = s.Set(1, chat.BitmapProactive).Set(1440, chat.BitmapProactiveMinutes).Set(30, chat.BitmapFollowupMinutes)
	c := chat.Storage(s)
	if c.Rate() != 100 || !c.NoAgent() || !c.Proactive() || c.ProactiveMinutes() != 1440 || c.FollowupMinutes() != 30 || s.Get(chat.BitmapTemp) != 81 {
		t.Fatal("overlapping settings")
	}
	if chat.Storage(0).Proactive() || chat.Storage(0).ProactiveMinutes() != 10 || chat.Storage(0).FollowupMinutes() != 2 {
		t.Fatal("defaults")
	}
}

func TestProactiveRelatedClassifierFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		output string
		status int
		want   bool
	}{
		{`{"related":true}`, 200, true}, {`{"related":false}`, 200, false},
		{`{"other":true}`, 200, false}, {"yes", 200, false}, {`{"related":true}`, 500, false},
	} {
		t.Run(tc.output+http.StatusText(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				raw, _ := json.Marshal(body)
				if !strings.Contains(string(raw), "recent_bot_conversation") || body["tools"] != nil {
					t.Error("missing context or classifier got tools")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": tc.output}}}})
			}))
			defer server.Close()
			cfg := chat.AgentConfig{API: server.URL, ModelName: "test", Key: "test-key"}
			if got := relatedWithClient(cfg, []string{"聊猫", "猫很可爱"}, "它吃什么", server.Client()); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestProactiveMentions(t *testing.T) {
	old := zero.BotConfig.NickName
	zero.BotConfig.NickName = []string{"测试机器人"}
	defer func() { zero.BotConfig.NickName = old }()
	for _, tc := range []struct {
		msg    message.Message
		direct bool
		want   bool
	}{
		{message.Message{message.Text("测试机器人你怎么看")}, false, true},
		{message.Message{message.At(123), message.Text("你好")}, false, true},
		{message.Message{message.At(456), message.Text("你好")}, false, false},
		{message.Message{message.Text("不相关的闲聊")}, true, true},
	} {
		ctx := &zero.Ctx{Event: &zero.Event{SelfID: 123, UserID: 456, IsToMe: tc.direct, Message: tc.msg}}
		if mentioned(ctx) != tc.want {
			t.Fatal("mention detection mismatch")
		}
	}
}

func TestProactiveAgentDeliveryWithNumericGroupAndMixedSegments(t *testing.T) {
	key := proactiveKey{67890123, 123456789}
	proactive.mu.Lock()
	delete(proactive.groups, key)
	proactive.mu.Unlock()
	ctx := &zero.Ctx{Event: &zero.Event{SelfID: key[0], GroupID: key[1], UserID: 123, Message: message.Message{message.Text("聊猫")}},
		State: zero.State{zero.StateKeyPrefixKeep + "aichatcfg_stor__": chat.Storage(chat.BitmapProactive)}}
	params := zero.Params{"group_id": float64(key[1]), "message": []any{
		map[string]any{"type": "at", "data": map[string]any{"qq": 123}},
		map[string]any{"type": "text", "data": map[string]any{"text": "猫很可爱"}},
	}}
	recordProactiveDelivery(ctx, "send_group_msg", params, zero.APIResponse{Status: "failed"})
	proactive.mu.Lock()
	failedRecorded := proactive.groups[key] != nil
	proactive.mu.Unlock()
	if failedRecorded {
		t.Fatal("failed delivery opened follow-up")
	}
	recordProactiveDelivery(ctx, "send_group_msg", params, zero.APIResponse{Status: "ok"})
	proactive.mu.Lock()
	s := proactive.groups[key]
	delete(proactive.groups, key)
	proactive.mu.Unlock()
	if s == nil || len(s.turns) != 2 || !strings.Contains(s.turns[1], "猫很可爱") {
		t.Fatal("numeric Agent group or mixed message lost")
	}
	if sameProactiveGroup(float64(627361951), key[1]) || sameProactiveGroup("bad", key[1]) {
		t.Fatal("wrong recipient accepted")
	}
}
