package minecraftobserver

import (
	"fmt"
	"testing"
)

func Test_PingListInfo(t *testing.T) {
	reachable := mockStatusServer(t)
	unreachable := closedStatusAddress(t)
	t.Run("normal", func(t *testing.T) {
		resp, err := getMinecraftServerStatus(reachable)
		if err != nil {
			t.Fatalf("getMinecraftServerStatus() error = %v", err)
		}
		msg, iconBase64 := resp.genServerSubscribeSchema(reachable, 123456).generateServerStatusMsg()
		fmt.Printf("msg: %v\n", msg)
		fmt.Printf("iconBase64: %v\n", iconBase64)
	})
	t.Run("不可达", func(t *testing.T) {
		ss, err := getMinecraftServerStatus(unreachable)
		if err == nil {
			t.Fatalf("getMinecraftServerStatus() error = %v", err)
		}
		if ss != nil {
			t.Fatalf("getMinecraftServerStatus() got = %v, want nil", ss)
		}
	})
}
