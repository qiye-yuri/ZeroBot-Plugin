//go:build ignore

package main

import (
	"bytes"
	"context"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/updatewatch"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func must(e error) {
	if e != nil {
		panic(e)
	}
}
func main() {
	if !run() {
		os.Exit(1)
	}
}
func run() bool {
	root, e := os.Getwd()
	must(e)
	must(os.MkdirAll(".local/notice-acceptance", 0700))
	out, e := os.MkdirTemp(filepath.Join(root, ".local/notice-acceptance"), "run-")
	must(e)
	fmt.Println("ARTIFACTS=" + out)
	h, e := qa.New(root, filepath.Join(root, ".local/bin/zerobot-notice-acceptance-final.exe"), out)
	must(e)
	defer h.Close()
	config := core.DefaultConfig()
	config.Enabled = false
	config.OwnerID = qa.Owner
	config.BotID = 87654321
	cp := filepath.Join(out, ".local/updatewatch/config.json")
	sp := filepath.Join(out, ".local/updatewatch/state.json")
	must(core.Save(cp, config))
	must(core.Save(sp, core.State{Seen: map[string]core.Event{}, Initialized: map[string]bool{}, Pending: []core.Notice{{Keys: []string{"pending-fixture"}, Text: "must remain queued"}}}))
	beforeC, e := os.ReadFile(cp)
	must(e)
	beforeS, e := os.ReadFile(sp)
	must(e)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	bot, e := h.Start(ctx, "notice")
	must(e)
	defer func() { bot.Stop() }()
	api := qa.NewClient(h.Address)
	_, e = api.Auth(h.AdminDir)
	must(e)
	toggle := func(mode string) {
		v, e := api.Checked("GET", "/api/plugins", nil)
		must(e)
		for _, raw := range v["plugins"].([]any) {
			p := raw.(map[string]any)
			if p["name"] == "updatewatch" {
				_, e = api.Checked("POST", "/api/plugin-switch", map[string]any{"name": "updatewatch", "groupId": "0", "mode": mode, "revision": p["state"].(map[string]any)["revision"]})
				must(e)
				return
			}
		}
		panic("missing plugin")
	}
	passed := true
	checks := []map[string]any{}
	check := func(n string, ok bool) {
		passed = passed && ok
		checks = append(checks, map[string]any{"name": n, "passed": ok})
		fmt.Println(n, ok)
	}
	send := func(uid, gid int64, text string) string {
		c, stop := context.WithTimeout(ctx, 4*time.Second)
		defer stop()
		var r string
		var err error
		if gid == 0 {
			r, _, err = h.SendWithContext(c, uid, text)
		} else {
			r, err = h.SendGroupAs(c, uid, gid, text, "owner")
		}
		if err != context.DeadlineExceeded {
			must(err)
		}
		return r
	}
	toggle("enabled")
	r := send(qa.Owner, 0, "更新监控状态")
	check("status-query", strings.Contains(r, "更新监控：false") && strings.Contains(r, "待通知：1"))
	r = send(qa.Owner, 0, "更新监控配置")
	check("config-query", strings.Contains(r, "intervalHours"))
	r = send(qa.Owner, 0, "更新监控测试通知")
	check("test-notice-delivered", strings.Contains(r, "[更新监控测试通知]") && strings.Contains(r, "不代表真实"))
	check("standard-message-array", bytes.HasPrefix(bytes.TrimSpace(h.LastSentMessage()), []byte("[")))
	check("group-denied", send(qa.Owner, 101, "更新监控测试通知") == "")
	check("nonowner-denied", send(23456789, 0, "更新监控测试通知") == "")
	afterC, e := os.ReadFile(cp)
	must(e)
	afterS, e := os.ReadFile(sp)
	must(e)
	check("schedule-config-unchanged", bytes.Equal(beforeC, afterC))
	check("pending-history-unchanged", bytes.Equal(beforeS, afterS))
	bot.Stop()
	bot, e = h.Start(ctx, "restart")
	must(e)
	_, e = api.Auth(h.AdminDir)
	must(e)
	check("restart-pending-preserved", strings.Contains(send(qa.Owner, 0, "更新监控状态"), "待通知：1"))
	toggle("disabled")
	check("disabled-silent", send(qa.Owner, 0, "更新监控测试通知") == "")
	must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed, "checks": checks, "realQQMessagesSent": false}))
	return passed
}
