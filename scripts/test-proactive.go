//go:build ignore

// Isolated OneBot + model acceptance; no real QQ messages or credentials.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
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
	binary := flag.String("binary-name", "zerobot-ai-proactive.exe", "binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, err := os.Getwd()
	must(err)
	parent := filepath.Join(root, ".local", "proactive-acceptance")
	must(os.MkdirAll(parent, 0700))
	out, err := os.MkdirTemp(parent, "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	var answers, judgments atomic.Int64
	var slowNext, modelBusy atomic.Bool
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		must(json.NewDecoder(r.Body).Decode(&body))
		raw, _ := json.Marshal(body)
		content := "AUTO_REPLY"
		if strings.Contains(string(raw), "recent_bot_conversation") {
			judgments.Add(1)
			content = `{"related":false}`
			var prompt string
			if msgs, ok := body["messages"].([]any); ok && len(msgs) > 0 {
				last, _ := msgs[len(msgs)-1].(map[string]any)
				prompt, _ = last["content"].(string)
			}
			var data struct {
				Message string `json:"new_group_message"`
			}
			_ = json.Unmarshal([]byte(prompt[strings.LastIndex(prompt, "\n")+1:]), &data)
			if strings.Contains(data.Message, "FOLLOWUP") {
				content = `{"related":true}`
			}
		} else {
			answers.Add(1)
			if slowNext.Swap(false) {
				modelBusy.Store(true)
				time.Sleep(1200 * time.Millisecond)
				modelBusy.Store(false)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}}}})
	}))
	defer model.Close()
	h, err := qa.NewWithIdentity(root, filepath.Join(root, ".local/bin", *binary), out, 87654321)
	must(err)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	bot, err := h.Start(ctx, "proactive")
	must(err)
	defer bot.Stop()
	api := qa.NewClient(h.Address)
	_, err = api.Auth(h.AdminDir)
	must(err)
	get := func(path string) map[string]any { v, e := api.Checked("GET", path, nil); must(e); return v }
	post := func(path string, body any) map[string]any { v, e := api.Checked("POST", path, body); must(e); return v }
	for _, v := range get("/api/plugins")["plugins"].([]any) {
		p := v.(map[string]any)
		if p["name"] == "aichat" {
			post("/api/plugin-switch", map[string]any{"name": "aichat", "groupId": "0", "mode": "enabled", "revision": p["state"].(map[string]any)["revision"]})
		}
	}
	v := get("/api/ai?group=101")
	post("/api/ai", map[string]any{"revision": v["global"].(map[string]any)["revision"], "fields": map[string]any{"API": model.URL, "ModelName": "fixture", "Type": 0}, "keys": map[string]any{"Key": map[string]any{"action": "replace", "value": "synthetic-key"}}})
	for _, raw := range get("/api/ai?group=101")["groups"].([]any) {
		g := raw.(map[string]any)
		if g["service"] == "aichatcfg" {
			post("/api/ai-group", map[string]any{"service": "aichatcfg", "groupId": "101", "revision": g["revision"], "useAgent": false, "voiceOutput": false, "respondAt": false, "rate": 0, "proactive": true, "proactiveMinutes": 1, "followupMinutes": 2})
		}
	}
	rows := []map[string]any{}
	check := func(name string, pass bool) {
		rows = append(rows, map[string]any{"name": name, "passed": pass})
		fmt.Printf("%s=%v\n", name, pass)
	}
	passed := false
	defer func() {
		must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed, "checks": rows, "isolated": true, "realQQMessagesSent": false, "modelAnswers": answers.Load(), "relevanceChecks": judgments.Load()}))
	}()
	send := func(uid int64, text, role string, timeout time.Duration, segments ...map[string]any) string {
		c, stop := context.WithTimeout(ctx, timeout)
		defer stop()
		reply, e := h.SendGroupAs(c, uid, 101, text, role, segments...)
		if e != context.DeadlineExceeded {
			must(e)
		}
		return reply
	}
	check("initial-message-starts-clock", send(qa.Owner, "INITIAL_TOPIC", "owner", time.Second) == "")
	startAnswers := answers.Load()
	// Wall time is intentional: verifies a real process honors a one-minute
	// configuration and does not post while the group has no new messages.
	time.Sleep(61 * time.Second)
	check("idle-group-no-unsolicited-post", answers.Load() == startAnswers)
	check("due-new-message-replies", send(qa.Owner, "DUE_TOPIC", "owner", 4*time.Second) == "AUTO_REPLY")
	time.Sleep(100 * time.Millisecond)
	check("related-followup-without-mention", send(23456789, "FOLLOWUP", "member", 4*time.Second) == "AUTO_REPLY")
	time.Sleep(100 * time.Millisecond)
	check("unrelated-keeps-quiet", send(23456789, "UNRELATED", "member", time.Second) == "")
	before := answers.Load()
	judged := judgments.Load()
	check("own-message-ignored", send(87654321, "本机测试 FOLLOWUP", "owner", time.Second) == "")
	check("own-message-no-model-call", answers.Load() == before && judgments.Load() == judged)
	check("nickname-bypasses-interval", send(qa.Owner, "本机测试 NICKNAME", "owner", 4*time.Second) == "AUTO_REPLY")
	time.Sleep(100 * time.Millisecond)
	check("at-bypasses-interval", send(qa.Owner, "MENTION", "owner", 4*time.Second,
		map[string]any{"type": "at", "data": map[string]string{"qq": "87654321"}},
		map[string]any{"type": "text", "data": map[string]string{"text": "MENTION"}}) == "AUTO_REPLY")
	time.Sleep(100 * time.Millisecond)
	before = answers.Load()
	slowNext.Store(true)
	first := make(chan string, 1)
	go func() { first <- send(qa.Owner, "本机测试 SLOW", "owner", 4*time.Second) }()
	deadline := time.Now().Add(3 * time.Second)
	for !modelBusy.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	second := send(qa.Owner, "本机测试 BUSY", "owner", 4*time.Second)
	both := second + <-first
	check("nickname-while-busy-gets-notice", strings.Contains(both, "正在处理") && strings.Contains(both, "AUTO_REPLY") && answers.Load() == before+1)
	_ = send(23456789, "关闭AI主动接话", "member", time.Second)
	settings := get("/api/ai?group=101")["groups"].([]any)[0].(map[string]any)
	check("member-cannot-change-mode", settings["proactive"] == true)
	check("admin-can-disable", strings.Contains(send(qa.Owner, "关闭AI主动接话", "owner", 4*time.Second), "已保存"))
	before = answers.Load()
	check("disabled-no-followup", send(23456789, "FOLLOWUP", "member", time.Second) == "")
	check("disabled-no-model-call", answers.Load() == before)
	passed = true
	for _, row := range rows {
		if row["passed"] != true {
			passed = false
		}
	}
	if !passed {
		fmt.Println("FAILED")
	}
	return passed
}
