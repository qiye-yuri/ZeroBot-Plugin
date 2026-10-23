//go:build ignore

// Isolated request routing checks; never connects to real QQ.
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
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
	if !run() {
		os.Exit(1)
	}
}
func run() bool {
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(".local/event-acceptance", 0700))
	out, err := os.MkdirTemp(filepath.Join(root, ".local/event-acceptance"), "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin/zerobot-event-acceptance.exe"), out)
	must(err)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	bot, err := h.Start(ctx, "requests")
	must(err)
	defer func() { bot.Stop() }()
	api := qa.NewClient(h.Address)
	_, err = api.Auth(h.AdminDir)
	must(err)
	passed, completed := true, false
	checks := []map[string]any{}
	defer func() {
		must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed && completed, "checks": checks, "isolated": true, "realQQMessagesSent": false}))
	}()
	check := func(name string, ok bool) {
		passed = passed && ok
		checks = append(checks, map[string]any{"name": name, "passed": ok})
		fmt.Printf("%s=%v\n", name, ok)
	}
	send := func(uid, gid int64, cmd string) string {
		c, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		var r string
		var e error
		if gid == 0 {
			r, _, e = h.SendWithContext(c, uid, cmd)
		} else {
			r, e = h.SendGroupAs(c, uid, gid, cmd, "owner")
		}
		if e != context.DeadlineExceeded {
			must(e)
		}
		time.Sleep(2100 * time.Millisecond)
		return r
	}
	owner := func(gid int64, cmd string) string { return send(qa.Owner, gid, cmd) }
	switchMain := func(mode string) {
		v, e := api.Checked("GET", "/api/plugins", nil)
		must(e)
		for _, raw := range v["plugins"].([]any) {
			p := raw.(map[string]any)
			if p["name"] == "event" {
				_, e = api.Checked("POST", "/api/plugin-switch", map[string]any{"name": "event", "groupId": "0", "mode": mode, "revision": p["state"].(map[string]any)["revision"]})
				must(e)
				return
			}
		}
		panic("missing event")
	}
	switchMain("enabled")
	check("default-settings", strings.Contains(owner(0, "查看申请处理设置"), "自动同意申请：关闭"))
	token := func(kind, flag string) string { return kind + "_" + base64.RawURLEncoding.EncodeToString([]byte(flag)) }
	request := func(uid, gid int64, flag string) string {
		c, stop := context.WithTimeout(ctx, 8*time.Second)
		defer stop()
		r, e := h.SendRequest(c, uid, gid, flag)
		must(e)
		return r
	}
	opaque := "0018446744073709551615/opaque+中文"
	r := request(23456789, 0, opaque)
	check("opaque-friend-notification", strings.Contains(r, token("f", opaque)))
	check("no-auto-friend", len(h.RequestCalls()) == 0)
	r = owner(0, "同意申请"+token("f", opaque))
	calls := h.RequestCalls()
	check("manual-friend-exact-flag", strings.Contains(r, "已同意申请") && len(calls) == 1 && calls[0].Flag == opaque && calls[0].Approve)
	r = request(23456789, 101, opaque)
	check("invite-notification", strings.Contains(r, token("g", opaque)))
	r = owner(0, "拒绝邀请"+token("g", opaque)+" 测试拒绝")
	calls = h.RequestCalls()
	check("manual-invite", strings.Contains(r, "已拒绝邀请") && len(calls) == 2 && calls[1].Flag == opaque && !calls[1].Approve && calls[1].SubType == "invite")
	check("wrong-kind-rejected", strings.Contains(owner(0, "同意申请"+token("g", opaque)), "处理失败"))
	check("malformed-rejected", strings.Contains(owner(0, "同意申请f_!"), "处理失败"))
	check("member-denied", send(23456789, 0, "同意申请"+token("f", opaque)) == "")
	check("group-denied", owner(101, "同意申请"+token("f", opaque)) == "")
	check("no-unauthorized-api", len(h.RequestCalls()) == 2)
	h.FailAction("set_friend_add_request")
	check("manual-failure-honest", strings.Contains(owner(0, "同意申请"+token("f", opaque)), "处理失败"))
	check("auto-master-failure-honest", strings.Contains(request(qa.Owner, 0, opaque), "自动同意失败"))
	h.FailAction("")
	check("auto-master-default", strings.Contains(request(qa.Owner, 101, opaque), "已自动同意"))
	check("disable-master", strings.Contains(owner(0, "关闭自动同意主人"), "已设置"))
	count := len(h.RequestCalls())
	check("master-off", strings.Contains(request(qa.Owner, 0, opaque), "请在下方"))
	check("master-off-no-decision", len(h.RequestCalls()) == count)
	check("enable-friend", strings.Contains(owner(0, "开启自动同意申请"), "已设置"))
	check("auto-friend", strings.Contains(request(23456789, 0, opaque), "已自动同意"))
	check("enable-invite", strings.Contains(owner(0, "开启自动同意邀请"), "已设置"))
	check("auto-invite", strings.Contains(request(23456789, 101, opaque), "已自动同意"))
	bot.Stop()
	bot, err = h.Start(ctx, "restart")
	must(err)
	_, err = api.Auth(h.AdminDir)
	must(err)
	r = owner(0, "查看申请处理设置")
	check("settings-persist", strings.Contains(r, "自动同意申请：开启") && strings.Contains(r, "自动同意邀请：开启") && strings.Contains(r, "自动同意主人：关闭"))
	switchMain("disabled")
	check("plugin-off", owner(0, "查看申请处理设置") == "")
	completed = true
	return passed
}
