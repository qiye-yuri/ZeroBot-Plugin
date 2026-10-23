//go:build ignore

// Synthetic QQ with one public Bilibili API lookup; no model or account credentials.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	ctrl "github.com/FloatTech/zbpctrl"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	root, err := os.Getwd()
	must(err)
	out, err := os.MkdirTemp(filepath.Join(root, ".local/bili-upload-removal"), "public-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin/zerobot-no-upload.exe"), out)
	must(err)
	defer h.Close()
	m := ctrl.NewManager[any](filepath.Join(out, "data/control/plugins.db"))
	c := m.NewControl("bilibiliparse", nil)
	must(c.SetData(-qa.Owner, 0x20))
	must(m.D.Close())
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	bot, err := h.Start(ctx, "card")
	must(err)
	defer bot.Stop()
	api := qa.NewClient(h.Address)
	_, err = api.Auth(h.AdminDir)
	must(err)
	plugins, err := api.Checked("GET", "/api/plugins", nil)
	must(err)
	publicHelp, _ := json.Marshal(plugins)
	checks := []map[string]any{}
	passed := true
	check := func(name string, ok bool) {
		checks = append(checks, map[string]any{"name": name, "passed": ok})
		passed = passed && ok
		fmt.Println(name, ok)
	}
	check("help-without-upload", !strings.Contains(string(publicHelp), "视频上传"))
	before := h.ActionCount("send_private_msg")
	reply, _, err := h.SendWithContext(ctx, qa.Owner, "https://www.bilibili.com/video/BV1xx411c7mD")
	must(err)
	check("public-card-success", reply != "" && !strings.Contains(reply, "ERROR:"))
	check("cover-present", h.ImageReceived)
	time.Sleep(5 * time.Second)
	message := h.LastSentMessage()
	check("retired-bit-no-extra-send", h.ActionCount("send_private_msg") == before+1)
	check("no-video-segment", !strings.Contains(string(message), `"type":"video"`))
	must(os.WriteFile(filepath.Join(out, "message.json"), message, 0600))
	must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed, "checks": checks, "realQQMessagesSent": false, "paidCalls": 0, "realPublicAPIs": true}))
	if !passed {
		bot.Stop()
		h.Close()
		os.Exit(1)
	}
}
