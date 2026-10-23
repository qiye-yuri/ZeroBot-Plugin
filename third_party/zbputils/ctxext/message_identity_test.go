package ctxext

import (
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
	"testing"
)

func TestBotForwardIdentity(t *testing.T) {
	old := zero.BotConfig.NickName
	defer func() { zero.BotConfig.NickName = old }()
	for _, names := range [][]string{nil, {"", "测试机器人"}} {
		zero.BotConfig.NickName = names
		node := BotForwardNode(&zero.Ctx{Event: &zero.Event{SelfID: 87654321, UserID: 12345678}}, message.Text("generated"))
		if node.Data["uin"] != "87654321" || node.Data["name"] == "" {
			t.Fatalf("wrong identity: %v", node.Data)
		}
	}
}
