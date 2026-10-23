//go:build ignore

// Isolated text-plugin checks with real public dependencies; no real QQ sends.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if !run() {
		os.Exit(1)
	}
}
func run() bool {
	binaryName := flag.String("binary-name", "zerobot.exe", "binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binaryName) != *binaryName {
		panic("invalid binary name")
	}
	root, e := os.Getwd()
	must(e)
	parent := filepath.Join(root, ".local/plugin-text")
	must(os.MkdirAll(parent, 0700))
	out, e := os.MkdirTemp(parent, "run-")
	must(e)
	fmt.Println("ARTIFACTS=" + out)
	bin := filepath.Join(root, ".local/bin", *binaryName)
	b, e := os.ReadFile(bin)
	must(e)
	sum := sha256.Sum256(b)
	h, e := qa.New(root, bin, out)
	must(e)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	bot, e := h.Start(ctx, "text")
	must(e)
	defer bot.Stop()
	rows := []map[string]any{}
	report := map[string]any{"binarySHA256": hex.EncodeToString(sum[:]), "isolated": true, "realQQMessagesSent": false, "dailyConfigurationWrites": false}
	defer func() { report["checks"] = rows; must(qa.WriteJSON(filepath.Join(out, "result.json"), report)) }()
	cases := []struct{ plugin, cmd, want string }{
		{"score", "查看我的钱包", "钱包有0"}, {"score", "查看钱包余额invalid", "QQ号处理失败"}}
	passed := true
	for _, c := range cases {
		wait, stop := context.WithTimeout(ctx, 35*time.Second)
		reply, elapsed, err := h.SendWithContext(wait, qa.Owner, c.cmd)
		stop()
		ok := err == nil && strings.Contains(reply, c.want)
		passed = passed && ok
		rows = append(rows, map[string]any{"plugin": c.plugin, "command": c.cmd, "reply": reply, "passed": ok, "seconds": elapsed.Seconds(), "error": fmt.Sprint(err)})
		fmt.Printf("%s passed=%v seconds=%.1f\n", c.plugin, ok, elapsed.Seconds())
		if err != nil {
			report["abortedAfterTimeout"] = true
			break
		}
	}
	report["passed"] = passed
	return passed
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
