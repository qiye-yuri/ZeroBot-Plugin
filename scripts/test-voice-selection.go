//go:build ignore

// Voice setup tests use a private synthetic OneBot instance and synthetic voices.
package main

import (
	"context"
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
	must(os.MkdirAll(".local/voice-selection", 0700))
	out, err := os.MkdirTemp(filepath.Join(root, ".local/voice-selection"), "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin/zerobot-voice-selection.exe"), out)
	must(err)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	bot, err := h.Start(ctx, "voice")
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
	send := func(uid, gid int64, text string) string {
		c, stop := context.WithTimeout(ctx, 4*time.Second)
		defer stop()
		var r string
		var e error
		if gid == 0 {
			r, _, e = h.SendWithContext(c, uid, text)
		} else {
			r, e = h.SendGroupAs(c, uid, gid, text, "member")
		}
		if e != context.DeadlineExceeded {
			must(e)
		}
		time.Sleep(1100 * time.Millisecond)
		return r
	}
	owner := func(text string) string { return send(qa.Owner, 0, text) }
	switchPlugin := func(name, mode string) {
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
	switchPlugin("airecord", "enabled")
	check("empty-catalogue", strings.Contains(owner("设置AI语音模型"), "无法获取"))
	models := []any{map[string]any{"characters": []any{map[string]any{"character_name": "合成甲", "character_id": "voice-a"}, map[string]any{"character_name": "合成乙", "character_id": "voice-b", "preview_url": "https://example.invalid/unsolicited-preview.mp3"}, map[string]any{"character_name": "重复甲", "character_id": "voice-a"}}}}
	h.SetVoiceModels(models)
	before := owner("查看AI语音配置")
	r := owner("设置AI语音模型")
	check("list-deduplicated", strings.Contains(r, "0. 合成甲") && strings.Contains(r, "1. 合成乙") && !strings.Contains(r, "重复甲"))
	check("other-user-isolated", send(23456789, 0, "1") == "")
	check("other-group-isolated", send(qa.Owner, 101, "1") == "")
	check("invalid-number", strings.Contains(owner("999"), "序号非法"))
	check("invalid-text", strings.Contains(owner("非序号"), "请输入数字"))
	check("cancel", strings.Contains(owner("取消"), "已取消"))
	check("cancel-keeps-config", owner("查看AI语音配置") == before)
	switchPlugin("aichat", "enabled")
	check("ai-enabled-selection-start", strings.Contains(owner("设置AI语音模型"), "合成乙"))
	check("ai-does-not-steal-selection", strings.Contains(owner(" 1 "), "已选择语音模型: 合成乙"))
	after := owner("查看AI语音配置")
	check("selected-config", strings.Contains(after, "合成乙"))
	check("ai-enabled-cancel-start", strings.Contains(owner("设置AI语音模型"), "合成乙"))
	check("ai-does-not-steal-cancel", strings.Contains(owner("取消"), "已取消"))
	check("cancel-preserves-selection", owner("查看AI语音配置") == after)
	switchPlugin("aichat", "disabled")
	check("nonowner-config-denied", send(23456789, 0, "设置AI语音模型") == "")
	check("group-config-denied", send(qa.Owner, 101, "设置AI语音模型") == "")
	count := h.ActionCount("send_group_ai_record")
	check("private-no-group-leak", strings.Contains(owner("发送AI语音私聊测试"), "仅支持群聊") && h.ActionCount("send_group_ai_record") == count)
	check("empty-voice-text", strings.Contains(owner("发送AI语音"), "填写"))
	// Observe past the old timeout to catch a late cancellation/error or AI reply.
	fmt.Println("waiting-for-late-reply-check")
	c, stop := context.WithTimeout(ctx, 125*time.Second)
	late, e := h.AwaitReply(c)
	stop()
	check("no-late-timeout-or-ai-response", e == context.DeadlineExceeded && late == "")
	bot.Stop()
	bot, err = h.Start(ctx, "restart")
	must(err)
	_, err = api.Auth(h.AdminDir)
	must(err)
	check("model-persists", owner("查看AI语音配置") == after)
	switchPlugin("airecord", "disabled")
	check("plugin-off", owner("设置AI语音模型") == "")
	completed = true
	return passed
}
