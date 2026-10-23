//go:build ignore

// Verify removed driftbottle routes with synthetic group mentions only.
package main

import (
	"context"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	"os"
	"path/filepath"
	"time"
)

func main() {
	root, err := os.Getwd()
	must(err)
	out, err := os.MkdirTemp(filepath.Join(root, ".local/leisure-prune"), "group-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin/zerobot-leisure-prune.exe"), out)
	must(err)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	bot, err := h.Start(ctx, "group")
	must(err)
	defer bot.Stop()
	checks := []map[string]any{}
	passed := true
	for _, cmd := range []string{"pick", "throw 这是一条合成测试漂流瓶内容"} {
		c, stop := context.WithTimeout(ctx, 2*time.Second)
		segments := []map[string]any{{"type": "at", "data": map[string]string{"qq": "87654321"}}, {"type": "text", "data": map[string]string{"text": " " + cmd}}}
		reply, e := h.SendGroupAs(c, qa.Owner, 101, "[CQ:at,qq=87654321] "+cmd, "owner", segments...)
		stop()
		ok := e == context.DeadlineExceeded && reply == ""
		passed = passed && ok
		checks = append(checks, map[string]any{"command": cmd, "passed": ok})
	}
	must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed, "checks": checks, "realQQMessagesSent": false}))
	if !passed {
		panic("Removed group command replied")
	}
	fmt.Println("checks=2 passed=true")
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
