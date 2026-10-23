//go:build ignore

// Isolated routing and persistence checks; no real QQ or upstream requests.
package main

import (
	"context"

	"flag"
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
	server := flag.String("server", "", "authorized Java server address")
	binary := flag.String("binary-name", "zerobot.exe", "binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(filepath.Join(root, ".local/mc-routing"), 0700))
	out, err := os.MkdirTemp(filepath.Join(root, ".local/mc-routing"), "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
	must(err)
	defer h.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
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
		c, stop := context.WithTimeout(ctx, 12*time.Second)
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
		fmt.Printf("received reply: %d chars\n", len([]rune(r)))
		time.Sleep(2100 * time.Millisecond)
		return r
	}
	owner := func(gid int64, cmd string) string { return send(qa.Owner, gid, cmd, "owner") }
	switchMain := func(name, mode string) {
		v, e := api.Checked("GET", "/api/plugins", nil)
		must(e)
		for _, raw := range v["plugins"].([]any) {
			p := raw.(map[string]any)
			if p["name"] == name {
				_, e = api.Checked("POST", "/api/plugin-switch", map[string]any{"name": name, "groupId": "0", "mode": mode, "revision": p["state"].(map[string]any)["revision"]})
				must(e)
				return
			}
		}
		panic("missing main")
	}

	switchMain("aichat", "disabled")

	if *server == "" {
		panic("server required")
	}
	switchMain("minecraftobserver", "enabled")
	check("initial-empty", strings.Contains(owner(0, "mc服务器订阅列表"), "没有订阅"))
	check("query-real-server", strings.Contains(owner(0, "mc服务器状态 "+*server), "版本"))
	check("subscribe-private", strings.Contains(owner(0, "mc服务器添加订阅 "+*server), "订阅添加成功"))
	follow, stop := context.WithTimeout(ctx, 10*time.Second)
	initial, e := h.AwaitReply(follow)
	stop()
	must(e)
	check("initial-status", strings.Contains(initial, "版本"))
	check("private-list", strings.Contains(owner(0, "mc服务器订阅列表"), *server))
	check("group-isolated", strings.Contains(owner(101, "mc服务器订阅列表"), "没有订阅"))
	check("duplicate-rejected", strings.Contains(owner(0, "mc服务器添加订阅 "+*server), "已经存在"))
	check("global-list", strings.Contains(owner(0, "mc服务器全局订阅列表"), *server))
	check("cancel-private", strings.Contains(owner(0, "mc服务器取消订阅 "+*server), "取消订阅成功"))
	check("empty-after-cancel", strings.Contains(owner(0, "mc服务器订阅列表"), "没有订阅"))
	switchMain("minecraftobserver", "disabled")
	check("disabled-silent", owner(0, "mc服务器订阅列表") == "")
	completed = true
	return passed
}
