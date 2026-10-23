//go:build ignore

// Voice configuration checks on synthetic accounts only.
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
	binary := flag.String("binary-name", "zerobot-repeat-boundaries.exe", "binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, e := os.Getwd()
	must(e)
	parent := filepath.Join(root, ".local/voice-config-boundaries")
	must(os.MkdirAll(parent, 0700))
	out, e := os.MkdirTemp(parent, "run-")
	must(e)
	fmt.Println("ARTIFACTS=" + out)
	h, e := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
	must(e)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	bot, e := h.Start(ctx, "voice-config")
	must(e)
	defer func() { bot.Stop() }()
	api := qa.NewClient(h.Address)
	_, e = api.Auth(h.AdminDir)
	must(e)
	toggle := func(name, mode string) {
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
		panic("missing plugin")
	}
	toggle("aichat", "disabled")
	toggle("airecord", "enabled")
	passed, complete := true, false
	checks := []map[string]any{}
	defer func() {
		must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed && complete, "checks": checks, "isolated": true, "realQQMessagesSent": false}))
	}()
	check := func(name string, ok bool) {
		passed = passed && ok
		checks = append(checks, map[string]any{"name": name, "passed": ok})
		fmt.Println(name, ok)
	}
	send := func(uid, gid int64, text string) string {
		c, stop := context.WithTimeout(ctx, 2*time.Second)
		defer stop()
		var r string
		var e error
		if gid == 0 {
			r, _, e = h.SendWithContext(c, uid, text)
		} else {
			r, e = h.SendGroupAs(c, uid, gid, text, "admin")
		}
		if e != context.DeadlineExceeded {
			must(e)
		}
		return r
	}
	owner := func(text string) string { return send(qa.Owner, 0, text) }
	before := owner("查看AI语音配置")
	check("baseline-readable", before != "")
	for _, value := range []string{"", "abc", "0", "-1", "1.5", "9223372036854775808"} {
		r := owner("设置AI语音群号" + value)
		check("invalid-gid-"+value, strings.Contains(r, "ERROR") || strings.Contains(r, "正数群号"))
	}
	check("invalid-preserves-config", owner("查看AI语音配置") == before)
	check("nonowner-write-denied", send(23456789, 0, "设置AI语音群号101") == "")
	check("nonowner-read-denied", send(23456789, 0, "查看AI语音配置") == "")
	check("group-write-denied", send(qa.Owner, 101, "设置AI语音群号101") == "")
	check("group-read-denied", send(qa.Owner, 101, "查看AI语音配置") == "")
	check("denials-preserve-config", owner("查看AI语音配置") == before)
	check("trimmed-valid-gid", strings.Contains(owner("设置AI语音群号 101 "), "设置AI语音群号为101"))
	after := owner("查看AI语音配置")
	check("config-shows-gid", strings.Contains(after, "101"))
	bot.Stop()
	bot, e = h.Start(ctx, "voice-config-restart")
	must(e)
	check("restart-preserves-gid", owner("查看AI语音配置") == after)
	check("no-voice-generated", h.ActionCount("send_group_ai_record") == 0)
	bot.Stop()
	for _, phase := range []string{"voice-config", "voice-config-restart"} {
		logs, e := os.ReadFile(filepath.Join(out, phase+"-stderr.log"))
		must(e)
		check(phase+"-no-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
	}
	complete = true
	return passed
}
