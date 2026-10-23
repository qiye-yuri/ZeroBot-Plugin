//go:build ignore

// Synthetic QQ with one public VUP lookup; no model or account credentials.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	ctrl "github.com/FloatTech/zbpctrl"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	binary := flag.String("binary-name", "zerobot-bili-select.exe", "binary filename")
	extended := flag.Bool("timeout", false, "also verify fixed deadline after an invalid choice")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(filepath.Join(root, ".local/bili-vup"), 0700))
	out, err := os.MkdirTemp(filepath.Join(root, ".local/bili-vup"), "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
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
	var modelCalls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"AI_SANITY"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()
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
	for _, raw := range plugins["plugins"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "aichat" {
			_, err = api.Checked("POST", "/api/plugin-switch", map[string]any{"name": "aichat", "groupId": "0", "mode": "enabled", "revision": p["state"].(map[string]any)["revision"]})
			must(err)
		}
	}
	private := fmt.Sprint(qa.Owner)
	v, err := api.Checked("GET", "/api/ai?private="+private, nil)
	must(err)
	_, err = api.Checked("POST", "/api/ai", map[string]any{"revision": v["global"].(map[string]any)["revision"], "fields": map[string]any{"API": srv.URL, "ModelName": "fixture", "Type": 0}, "keys": map[string]any{"Key": map[string]any{"action": "replace", "value": "synthetic-key"}}})
	must(err)
	v, err = api.Checked("GET", "/api/ai?private="+private, nil)
	must(err)
	for _, raw := range v["groups"].([]any) {
		g := raw.(map[string]any)
		if g["service"] == "aichatcfg" {
			_, err = api.Checked("POST", "/api/ai-group", map[string]any{"service": "aichatcfg", "privateId": private, "revision": g["revision"], "useAgent": false, "voiceOutput": false, "respondAt": true, "rate": 100})
			must(err)
		}
	}
	sanity, _, err := h.SendWithContext(ctx, qa.Owner, "fixture-AI-sanity")
	must(err)
	check("ai-active", strings.Contains(sanity, "AI_SANITY") && modelCalls.Load() > 0)
	time.Sleep(200 * time.Millisecond)
	modelsBefore := modelCalls.Load()
	before := h.ActionCount("send_private_msg")
	reply, _, err := h.SendWithContext(ctx, qa.Owner, ">vup info 672328094")
	must(err)
	check("numeric-choice-prompt", strings.Contains(reply, "0. 查询uid"))
	reply, _, err = h.SendWithContext(ctx, qa.Owner, "0")
	must(err)
	check("vup-identity", strings.Contains(reply, "672328094") && strings.Contains(reply, "嘉然"))
	check("vup-fields", strings.Contains(reply, "当前粉丝数:") && strings.Contains(reply, "直播间id:"))
	check("vup-source", strings.Contains(reply, "https://vtbs.moe/detail/672328094"))
	check("no-error", !strings.Contains(reply, "ERROR:"))
	check("prompt-and-result-only", h.ActionCount("send_private_msg") == before+2)
	message := h.LastSentMessage()
	check("choice-not-consumed-by-ai", modelCalls.Load() == modelsBefore)
	count := h.ActionCount("send_private_msg")
	time.Sleep(12 * time.Second)
	check("no-late-timeout", h.ActionCount("send_private_msg") == count)

	reply, _, err = h.SendWithContext(ctx, qa.Owner, ">vup info 672328094")
	must(err)
	check("reopen-menu", strings.Contains(reply, "0. 查询uid"))
	reply, _, err = h.SendWithContext(ctx, qa.Owner, "2")
	must(err)
	check("invalid-choice-local", strings.Contains(reply, "请输入0、1"))
	reply, _, err = h.SendWithContext(ctx, qa.Owner, "取消")
	must(err)
	check("cancel-menu", strings.Contains(reply, "已取消B站用户查询"))
	count = h.ActionCount("send_private_msg")
	time.Sleep(11 * time.Second)
	check("cancel-no-late-timeout", h.ActionCount("send_private_msg") == count)
	check("menu-no-ai", modelCalls.Load() == modelsBefore)
	if *extended {
		reply, _, err = h.SendWithContext(ctx, qa.Owner, ">vup info 672328094")
		must(err)
		check("timeout-menu", strings.Contains(reply, "0. 查询uid"))
		started := time.Now()
		time.Sleep(3 * time.Second)
		reply, _, err = h.SendWithContext(ctx, qa.Owner, "2")
		must(err)
		check("timeout-invalid-choice", strings.Contains(reply, "请输入0、1"))
		reply, err = h.AwaitReply(ctx)
		must(err)
		elapsed := time.Since(started)
		check("fixed-deadline", strings.Contains(reply, "选择超时") && elapsed >= 9*time.Second && elapsed < 12*time.Second)
		reply, err = h.AwaitReply(ctx)
		must(err)
		check("timeout-still-queries-uid", strings.Contains(reply, "672328094") && strings.Contains(reply, "嘉然"))
		check("timeout-no-ai", modelCalls.Load() == modelsBefore)
	}
	must(os.WriteFile(filepath.Join(out, "message.json"), message, 0600))
	must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed, "checks": checks, "realQQMessagesSent": false, "paidCalls": 0, "realPublicAPIs": true}))
	if !passed {
		bot.Stop()
		h.Close()
		os.Exit(1)
	}
}
