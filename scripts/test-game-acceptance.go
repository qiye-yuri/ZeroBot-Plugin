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
	binary := flag.String("binary-name", "zerobot.exe", "binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(filepath.Join(root, ".local/game-acceptance"), 0700))
	out, err := os.MkdirTemp(filepath.Join(root, ".local/game-acceptance"), "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
	must(err)
	defer h.Close()
	must(os.MkdirAll(filepath.Join(out, "data/Tarot"), 0700))
	for _, name := range []string{"tarots.json", "formation.json"} {
		raw, e := os.ReadFile(filepath.Join(root, "data/Tarot", name))
		must(e)
		must(os.WriteFile(filepath.Join(out, "data/Tarot", name), raw, 0600))
	}

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
		c, stop := context.WithTimeout(ctx, 5*time.Second)
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
		fmt.Printf("reply %s: %s\n", cmd, r)
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
	for _, name := range []string{"mcfish", "qqwife", "tarot"} {
		switchMain(name, "enabled")
	}
	check("fish-rules", strings.Contains(owner(0, "查看钓鱼规则"), "钓鱼"))
	r := owner(0, "当前装备概率明细")
	check("fish-probability", strings.Contains(r, "概率") && !strings.Contains(r, "ERROR"))
	check("marriage-empty", strings.Contains(owner(101, "群老婆列表"), "没有人结婚"))
	check("marriage-private-silent", owner(0, "群老婆列表") == "")
	check("reset-member-denied", send(23456789, 101, "重置花名册", "member") == "")
	r = owner(101, "好感度列表")
	check("favor-empty", r != "" && !strings.Contains(r, "ERROR"))
	check("tarot-zero", strings.Contains(owner(0, "抽0张塔罗牌"), "张数必须为正"))
	check("tarot-too-many", strings.Contains(owner(0, "抽21张塔罗牌"), "抽取张数过多"))
	check("formation-invalid", strings.Contains(owner(0, "塔罗牌阵不存在"), "现有牌阵列表"))
	for _, item := range []struct{ name, cmd string }{{"mcfish", "查看钓鱼规则"}, {"qqwife", "群老婆列表"}, {"tarot", "抽0张塔罗牌"}} {
		switchMain(item.name, "disabled")
		check(item.name+"-off", owner(101, item.cmd) == "")
	}
	completed = true
	return passed
}
