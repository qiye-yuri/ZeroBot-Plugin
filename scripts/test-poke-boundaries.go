//go:build ignore

// Synthetic notifications only; no real QQ or model requests.
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
	parent := filepath.Join(root, ".local/poke-boundaries")
	must(os.MkdirAll(parent, 0700))
	out, e := os.MkdirTemp(parent, "run-")
	must(e)
	fmt.Println("ARTIFACTS=" + out)
	h, e := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
	must(e)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	bot, e := h.Start(ctx, "poke")
	must(e)
	defer bot.Stop()
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
	toggle("chat", "enabled")
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
	poke := func(gid, target int64) string {
		c, stop := context.WithTimeout(ctx, 2500*time.Millisecond)
		defer stop()
		r, e := h.SendPokeTarget(c, qa.Owner, gid, target)
		if e != context.DeadlineExceeded {
			must(e)
		}
		return r
	}
	check("other-target-ignored", poke(101, 23456789) == "")
	for i := 1; i <= 4; i++ {
		check(fmt.Sprintf("group-first-four-%d", i), strings.Contains(poke(101, 87654321), "戳"))
	}
	check("group-fifth-rate-limited", poke(101, 87654321) == "")
	check("other-group-unaffected", strings.Contains(poke(102, 87654321), "戳"))
	check("private-independent-from-group", strings.Contains(poke(0, 87654321), "戳"))
	toggle("chat", "disabled")
	check("disabled-group-silent", poke(103, 87654321) == "")
	check("disabled-private-silent", poke(0, 87654321) == "")
	toggle("chat", "enabled")
	check("reenabled-group-replies", strings.Contains(poke(103, 87654321), "戳"))
	c, stop := context.WithTimeout(ctx, 2500*time.Millisecond)
	_, e = h.AwaitReply(c)
	stop()
	check("no-delayed-reply", e == context.DeadlineExceeded)
	bot.Stop()
	logs, e := os.ReadFile(filepath.Join(out, "poke-stderr.log"))
	must(e)
	check("no-handler-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
	complete = true
	return passed
}
