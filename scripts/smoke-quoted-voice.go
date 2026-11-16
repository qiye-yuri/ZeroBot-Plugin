//go:build ignore

// Integration replay using a synthetic OneBot, a temporary database and a local model.
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

func check(err error) {
	if err != nil {
		panic(err)
	}
}
func require(ok bool, msg string) {
	if !ok {
		panic(msg)
	}
}
func main() {
	resourceRoot := flag.String("resource-root", "", "Optional checkout containing the static fixture fonts and images; no runtime config is copied.")
	flag.Parse()
	root, err := os.Getwd()
	check(err)
	if *resourceRoot == "" {
		*resourceRoot = root
	}
	dir, err := os.MkdirTemp(filepath.Join(root, ".local"), "quoted-voice-")
	check(err)
	h, err := qa.NewWithIdentity(*resourceRoot, filepath.Join(root, ".local/bin/zerobot-quoted-voice-candidate.exe"), dir, 87654321)
	check(err)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	bot, err := h.Start(ctx, "voice")
	check(err)
	defer bot.Stop()
	var calls atomic.Int32
	const transcript = "活动从周六改到周日"
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string
			Messages []struct {
				Role    string
				Content any
			}
		}
		check(json.NewDecoder(r.Body).Decode(&body))
		raw, _ := json.Marshal(body.Messages)
		answer := "请求缺少当前群角色或引用内容"
		if body.Model == "fixture-agent" && strings.Contains(string(raw), "鲸鱼设定") && strings.Contains(string(raw), transcript) && strings.Contains(string(raw), "最后是哪天") {
			answer = "周日"
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": answer}}}})
	}))
	defer model.Close()
	client := qa.NewClient(h.Address)
	_, err = client.Auth(h.AdminDir)
	check(err)
	api := func(method, path string, body any) map[string]any {
		v, e := client.Checked(method, path, body)
		check(e)
		return v
	}
	for _, row := range api("GET", "/api/plugins", nil)["plugins"].([]any) {
		p := row.(map[string]any)
		if p["name"] == "aichat" {
			api("POST", "/api/plugin-switch", map[string]any{"name": "aichat", "groupId": "0", "mode": "enabled", "revision": p["state"].(map[string]any)["revision"]})
		}
	}
	v := api("GET", "/api/ai?group=101", nil)
	key := map[string]string{"action": "replace", "value": "fixture-key"}
	api("POST", "/api/ai", map[string]any{"revision": v["global"].(map[string]any)["revision"], "fields": map[string]any{"API": model.URL, "ModelName": "fixture", "Type": 0, "AgentAPI": model.URL, "AgentModelName": "fixture-agent", "AgentType": 0}, "keys": map[string]any{"Key": key, "AgentKey": key}})
	for _, row := range api("GET", "/api/ai?group=101", nil)["groups"].([]any) {
		g := row.(map[string]any)
		if g["service"] == "aichatcfg" {
			api("POST", "/api/ai-group", map[string]any{"service": "aichatcfg", "groupId": "101", "revision": g["revision"], "useAgent": true, "voiceOutput": true, "respondAt": true, "proactive": false, "rate": 0})
		}
	}
	p := api("GET", "/api/ai-persona?botId=87654321&type=group&id=101", nil)
	api("POST", "/api/ai-persona", map[string]any{"botId": "87654321", "type": "group", "id": "101", "revision": p["revision"], "mode": "custom", "character": "鲸鱼设定"})
	seg := func(typ, k, val string) map[string]any {
		return map[string]any{"type": typ, "data": map[string]string{k: val}}
	}
	fixture := func(group int64, text string) {
		check(h.SetMessageFixture(901, map[string]any{"group_id": group, "message_id": 901, "message": []any{seg("record", "file", "fixture.amr")}, "fixture_transcript": text}))
	}
	send := func(input string, at bool) string {
		segs := []map[string]any{seg("reply", "id", "901")}
		if at {
			segs = append(segs, seg("at", "qq", "87654321"))
		}
		segs = append(segs, seg("text", "text", input))
		c, stop := context.WithTimeout(ctx, 4*time.Second)
		defer stop()
		reply, e := h.SendGroupAs(c, qa.Owner, 101, input, "owner", segs...)
		if e != context.DeadlineExceeded {
			check(e)
		}
		return reply
	}
	fixture(101, transcript)
	require(send("转文字", false) == "" && h.ActionCount("fetch_ptt_text") == 0, "unmentioned quote transcribed")
	require(send("转文字", true) == transcript && calls.Load() == 0, "transcript not returned directly")
	require(send("最后是哪天？", true) == "周日" && calls.Load() == 1 && h.ActionCount("fetch_ptt_text") == 2, "question or persona routing failed")
	fixture(202, transcript)
	require(strings.Contains(send("转文字", true), "本群") && h.ActionCount("fetch_ptt_text") == 2, "cross-group reference transcribed")
	fixture(101, "")
	require(strings.Contains(send("最后是哪天？", true), "未能转成文字") && calls.Load() == 1, "empty transcript reached model")
	fmt.Println("PASS: explicit mention, direct transcript, persona answer, per-request transcription count, group boundary, empty transcript")
}
