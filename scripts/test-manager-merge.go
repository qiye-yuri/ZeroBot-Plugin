//go:build ignore

// Isolated routing and persistence checks; no real QQ or upstream requests.
package main

import (
	"context"

	"flag"
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
	if !run() {
		os.Exit(1)
	}
}
func run() bool {
	binary := flag.String("binary-name", "zerobot-manager-merge.exe", "binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(filepath.Join(root, ".local/manager-merge"), 0700))
	out, err := os.MkdirTemp(filepath.Join(root, ".local/manager-merge"), "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
	must(err)
	defer h.Close()
	m := ctrl.NewManager[any](filepath.Join(out, "data/control/plugins.db"))
	legacy := m.NewControl("autowithdraw", nil)
	must(legacy.Disable(101))
	must(legacy.Enable(102))
	must(m.D.Close())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	bot, err := h.Start(ctx, "bili")
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
	send := func(uid, gid int64, cmd, role string) string {
		c, stop := context.WithTimeout(ctx, 1800*time.Millisecond)
		defer stop()
		var r string
		var e error
		if gid == 0 {
			r, _, e = h.SendWithContext(c, uid, cmd)
		} else {
			r, e = h.SendGroupAs(c, uid, gid, cmd, role)
		}
		if e != context.DeadlineExceeded {
			must(e)
		}
		return r
	}
	owner := func(gid int64, cmd string) string { return send(qa.Owner, gid, cmd, "owner") }
	switchMain := func(mode string) {
		v, e := api.Checked("GET", "/api/plugins", nil)
		must(e)
		for _, raw := range v["plugins"].([]any) {
			p := raw.(map[string]any)
			if p["name"] == "manager" {
				_, e = api.Checked("POST", "/api/plugin-switch", map[string]any{"name": "manager", "groupId": "0", "mode": mode, "revision": p["state"].(map[string]any)["revision"]})
				must(e)
				return
			}
		}
		panic("missing main")
	}

	switchMain("enabled")
	recall := func(gid int64, want bool) {
		// Wait for SendChain to finish storing its trigger association.
		time.Sleep(300 * time.Millisecond)
		c, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
		id, e := h.RecallLast(c, qa.Owner, gid)
		cancel()
		fmt.Printf("recall observation: group=%d id=%d err=%v expected=%v\n", gid, id, e, want)
		if want {
			check(fmt.Sprintf("recall-%d-enabled", gid), e == nil && id == 999)
		} else {
			check(fmt.Sprintf("recall-%d-disabled", gid), e == context.DeadlineExceeded)
		}
	}
	check("legacy-off-preserved", strings.Contains(owner(101, "查看群管功能开关"), "自动撤回：关闭"))
	recall(101, false)
	check("other-group-default-on", strings.Contains(owner(102, "查看群管功能开关"), "自动撤回：开启"))
	recall(102, true)
	check("member-denied", send(23456789, 101, "开启自动撤回", "member") == "")
	check("admin-enables", strings.Contains(send(23456789, 101, "开启自动撤回", "admin"), "已开启"))
	check("group-now-on", strings.Contains(owner(101, "查看群管功能开关"), "自动撤回：开启"))
	recall(101, true)
	check("private-default-on", strings.Contains(owner(0, "查看群管功能开关"), "自动撤回：开启"))
	recall(0, true)
	check("private-off", strings.Contains(owner(0, "关闭自动撤回"), "已关闭"))
	recall(0, false)
	check("private-does-not-disable-group", strings.Contains(owner(101, "查看群管功能开关"), "自动撤回：开启"))
	check("group-disable", strings.Contains(owner(101, "关闭自动撤回"), "已关闭"))
	// Read-only reminder listing proves the parent remains active.
	check("other-manager-command-active", owner(101, "列出所有提醒") != "")
	check("unaffected-group-on", strings.Contains(owner(102, "查看群管功能开关"), "自动撤回：开启"))
	switchMain("disabled")
	recall(102, false)
	check("master-disabled-command-silent", owner(0, "查看群管功能开关") == "")
	switchMain("enabled")
	check("master-keeps-subfeature-off", strings.Contains(owner(0, "查看群管功能开关"), "自动撤回：关闭"))
	for _, cmd := range []string{"获取BDAKey", "run合成注入标记"} {
		check("removed-"+cmd, owner(0, cmd) == "")
	}
	bot.Stop()
	bot, err = h.Start(ctx, "manager-restart")
	must(err)
	check("restart-private-off", strings.Contains(owner(0, "查看群管功能开关"), "自动撤回：关闭"))
	check("restart-group-off", strings.Contains(owner(101, "查看群管功能开关"), "自动撤回：关闭"))
	check("restart-other-group-on", strings.Contains(owner(102, "查看群管功能开关"), "自动撤回：开启"))
	bot.Stop()
	m = ctrl.NewManager[any](filepath.Join(out, "data/control/plugins.db"))
	legacy = m.NewControl("autowithdraw", nil)
	must(legacy.Disable(0))
	must(m.D.Close())
	bot, err = h.Start(ctx, "manager-legacy-global")
	must(err)
	check("legacy-global-off-preserved", strings.Contains(owner(102, "查看群管功能开关"), "自动撤回：关闭"))
	check("explicit-override-global", strings.Contains(owner(102, "开启自动撤回"), "已开启"))
	check("override-state", strings.Contains(owner(102, "查看群管功能开关"), "自动撤回：开启"))
	recall(102, true)
	check("other-scope-global-still-off", strings.Contains(owner(103, "查看群管功能开关"), "自动撤回：关闭"))
	completed = true
	return passed
}
