//go:build ignore

// Tests only synthetic group data and local images; never sends real QQ messages.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	ctrl "github.com/FloatTech/zbpctrl"
	"image"
	"image/png"
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
	if !run() {
		os.Exit(1)
	}
}
func run() bool {
	gameLifecycle := flag.Bool("game-lifecycle", false, "test guess outcomes, idle timeout and room release")
	uploadLifecycle := flag.Bool("upload-lifecycle", false, "test upload replacement, failure preservation and persistence")
	binary := flag.String("binary-name", "zerobot-picture-prune.exe", "binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, err := os.Getwd()
	must(err)
	parent := filepath.Join(root, ".local/wife-acceptance")
	must(os.MkdirAll(parent, 0700))
	out, err := os.MkdirTemp(parent, "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
	must(err)
	defer h.Close()
	var pic bytes.Buffer
	must(png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 64, 64))))
	write := func(name string, b []byte) {
		path := filepath.Join(out, name)
		must(os.MkdirAll(filepath.Dir(path), 0700))
		must(os.WriteFile(path, b, 0600))
	}
	write("data/nwife/2t/旧角色", pic.Bytes()) // base36 group 101
	write("data/Wife/wife.json", []byte(`["[测试作品]内置角色.png"]`))
	write("data/Wife/wives/[测试作品]内置角色.png", pic.Bytes())
	m := ctrl.NewManager[any](filepath.Join(out, "data/control/plugins.db"))
	legacy := m.NewControl("nwife", nil)
	must(legacy.SetData(102, 1))
	must(m.D.Close())
	var imageMode atomic.Int64
	var replacement bytes.Buffer
	must(png.Encode(&replacement, image.NewRGBA(image.Rect(0, 0, 32, 32))))
	var modelCalls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/model") {
			modelCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"AI_SANITY"},"finish_reason":"stop"}]}`)
			return
		}
		if imageMode.Load() == 1 {
			w.Write(replacement.Bytes())
			return
		}
		if imageMode.Load() == 2 {
			http.Error(w, "fixture failure", http.StatusBadGateway)
			return
		}
		if imageMode.Load() == 3 {
			fmt.Fprint(w, "not an image")
			return
		}
		w.Write(pic.Bytes())
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	bot, err := h.Start(ctx, "wife")
	must(err)
	defer bot.Stop()
	api := qa.NewClient(h.Address)
	_, err = api.Auth(h.AdminDir)
	must(err)
	checks := []map[string]any{}
	passed := true
	completed := false
	defer func() {
		must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed && completed, "checks": checks, "isolated": true, "realQQMessagesSent": false}))
	}()
	check := func(name string, ok bool) {
		checks = append(checks, map[string]any{"name": name, "passed": ok})
		passed = passed && ok
		fmt.Printf("%s=%v\n", name, ok)
	}
	send := func(uid, gid int64, text, role string, upload bool) string {
		c, stop := context.WithTimeout(ctx, 3*time.Second)
		defer stop()
		var seg []map[string]any
		if upload {
			seg = []map[string]any{{"type": "text", "data": map[string]string{"text": text}}, {"type": "image", "data": map[string]string{"file": "fixture.png", "url": srv.URL + "/image"}}}
		}
		r, e := h.SendGroupAs(c, uid, gid, text, role, seg...)
		if e != context.DeadlineExceeded {
			must(e)
		}
		return r
	}
	owner := func(gid int64, text string) string { return send(qa.Owner, gid, text, "owner", false) }
	if *gameLifecycle {
		exchange := func(cmd, want string) string {
			c, done := context.WithTimeout(ctx, 5*time.Second)
			defer done()
			r, _, e := h.SendWithContext(c, qa.Owner, cmd)
			if e != context.DeadlineExceeded {
				must(e)
			}
			for !strings.Contains(r, want) && e == nil {
				var next string
				next, e = h.AwaitReply(c)
				r += next
			}
			check(cmd+"->"+want, strings.Contains(r, want))
			return r
		}
		exchange("猜老婆", "请回答")
		exchange("不存在甲酱", "还有2次")
		exchange("不存在乙酱", "还有1次")
		exchange("不存在丙酱", "次数到了")
		exchange("猜老婆", "请回答")
		exchange("内置角色酱", "太棒了")
		exchange("猜老婆", "请回答")
		exchange("跳过。", "已跳过")
		exchange("猜老婆", "请回答")
		start := time.Now()
		seen := map[string]int{}
		deadline, stop := context.WithTimeout(ctx, 130*time.Second)
		for seen["timeout"] == 0 {
			reply, e := h.AwaitReply(deadline)
			if e == context.DeadlineExceeded {
				break
			}
			must(e)
			if strings.Contains(reply, "还有15s") {
				seen["reminder"]++
				check("reminder-not-early", time.Since(start) >= 100*time.Second)
			}
			if strings.Contains(reply, "倒计时结束") {
				seen["timeout"]++
				check("timeout-not-early", time.Since(start) >= 115*time.Second)
				check("timeout-reveals-role", strings.Contains(reply, "内置角色"))
			}
		}
		stop()
		check("reminder-once", seen["reminder"] == 1)
		check("timeout-once", seen["timeout"] == 1)
		exchange("猜老婆", "请回答")
		exchange("跳过", "已跳过")
		quiet, done := context.WithTimeout(ctx, 10*time.Second)
		reply, e := h.AwaitReply(quiet)
		done()
		check("quiet-after-exit", reply == "" && e == context.DeadlineExceeded)
		bot.Stop()
		logs, e := os.ReadFile(filepath.Join(out, "wife-stderr.log"))
		must(e)
		check("no-handler-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
		completed = true
		return passed
	}
	if *uploadLifecycle {
		target := filepath.Join(out, "data/nwife/2t/验收临时角色")
		check("upload-create", strings.Contains(send(qa.Owner, 101, "添加wife验收临时角色", "owner", true), "已保存"))
		data, err := os.ReadFile(target)
		must(err)
		check("original-image-bytes", bytes.Equal(data, pic.Bytes()))
		imageMode.Store(1)
		check("same-name-replace", strings.Contains(send(qa.Owner, 101, "添加wife验收临时角色", "owner", true), "已保存"))
		data, err = os.ReadFile(target)
		must(err)
		check("replacement-image-bytes", bytes.Equal(data, replacement.Bytes()))
		for _, mode := range []int64{2, 3} {
			imageMode.Store(mode)
			check(fmt.Sprintf("failed-upload-%d", mode), strings.Contains(send(qa.Owner, 101, "添加wife验收临时角色", "owner", true), "保存失败"))
			data, err = os.ReadFile(target)
			must(err)
			check(fmt.Sprintf("old-image-preserved-%d", mode), bytes.Equal(data, replacement.Bytes()))
		}
		// Sending the response precedes deferred file cleanup; wait for handler completion before restart.
		var temporary []string
		cleanupDeadline := time.Now().Add(2 * time.Second)
		for {
			temporary, err = filepath.Glob(filepath.Join(out, "data/nwife/2t/.upload-*"))
			must(err)
			if len(temporary) == 0 || time.Now().After(cleanupDeadline) {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		check("no-leftover-upload", len(temporary) == 0)
		_, err = os.Stat(filepath.Join(out, "data/nwife/2u/验收临时角色"))
		check("other-group-unchanged", os.IsNotExist(err))
		bot.Stop()
		bot, err = h.Start(ctx, "wife-restart")
		must(err)
		defer bot.Stop()
		data, err = os.ReadFile(target)
		must(err)
		check("image-survives-restart", bytes.Equal(data, replacement.Bytes()))
		check("delete-test-role", strings.Contains(owner(101, "删除wife验收临时角色"), "已删除"))
		_, err = os.Stat(target)
		check("deleted-file-gone", os.IsNotExist(err))
		data, err = os.ReadFile(filepath.Join(out, "data/nwife/2t/旧角色"))
		must(err)
		check("existing-role-preserved", bytes.Equal(data, pic.Bytes()))
		check("delete-missing-reported", strings.Contains(owner(101, "删除wife验收临时角色"), "删除失败"))
		bot.Stop()
		for _, name := range []string{"wife", "wife-restart"} {
			logs, err := os.ReadFile(filepath.Join(out, name+"-stderr.log"))
			must(err)
			check(name+"-no-handler-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
		}
		completed = true
		return passed
	}
	check("legacy-images-main-entry", strings.Contains(owner(101, "抽老婆"), "旧角色"))
	check("legacy-alias", strings.Contains(owner(101, "抽wife"), "旧角色"))
	check("other-group-empty", strings.Contains(owner(102, "抽wife"), "尚无"))
	check("builtin-explicit", strings.Contains(owner(101, "抽内置老婆"), "内置角色"))
	check("builtin-fallback", strings.Contains(owner(102, "抽老婆"), "内置角色"))
	check("member-upload-denied", send(23456789, 101, "添加wife未授权", "member", true) == "")
	_, err = os.Stat(filepath.Join(out, "data/nwife/2t/未授权"))
	check("denied-no-file", os.IsNotExist(err))
	check("legacy-open-permission", strings.Contains(send(23456789, 102, "添加wife新角色", "member", true), "已保存"))
	check("new-role-main-entry", strings.Contains(owner(102, "抽老婆"), "新角色"))
	check("owner-close-upload", strings.Contains(owner(102, "不让所有人均可添加wife"), "已保存"))
	check("closed-member-denied", send(23456789, 102, "添加wife另一个", "member", true) == "")
	check("admin-upload", strings.Contains(send(34567890, 101, "添加wife管理员角色", "admin", true), "已保存"))
	check("path-rejected", strings.Contains(send(qa.Owner, 101, "添加wife../escape", "owner", true), "无效"))
	check("member-delete-denied", send(23456789, 102, "删除wife新角色", "member", false) == "")
	check("admin-delete", strings.Contains(owner(102, "删除wife新角色"), "已删除"))
	check("fallback-after-delete", strings.Contains(owner(102, "抽老婆"), "内置角色"))
	check("daily-selection-stable", owner(101, "抽老婆") == owner(101, "抽老婆"))
	_, _, err = h.Send(qa.Owner, "猜老婆")
	must(err)
	time.Sleep(100 * time.Millisecond)
	r, _, err := h.Send(qa.Owner, "跳过")
	must(err)
	if !strings.Contains(r, "已跳过") {
		r, _, err = h.Send(qa.Owner, "fixture-drain")
		must(err)
	}
	check("guess-game-retained", strings.Contains(r, "已跳过") && strings.Contains(r, "内置角色"))
	v, err := api.Checked("GET", "/api/plugins", nil)
	must(err)
	for _, raw := range v["plugins"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "wife" {
			check("unified-help", strings.Contains(p["usage"].(string), "添加wife") && strings.Contains(p["usage"].(string), "猜老婆"))
			_, err = api.Checked("POST", "/api/plugin-switch", map[string]any{"name": "wife", "groupId": "0", "mode": "disabled", "revision": p["state"].(map[string]any)["revision"]})
			must(err)
		}
	}
	for _, cmd := range []string{"抽老婆", "抽wife", "抽内置老婆", "猜老婆", "删除wife旧角色", "让所有人均可添加wife"} {
		check("disabled-"+cmd, owner(101, cmd) == "")
	}
	check("disabled-upload", send(qa.Owner, 101, "添加wife关闭测试", "owner", true) == "")
	// Regression for real private messages ending in punctuation, with AI enabled.
	for _, service := range []string{"wife", "aichat"} {
		v, err = api.Checked("GET", "/api/plugins", nil)
		must(err)
		for _, raw := range v["plugins"].([]any) {
			p := raw.(map[string]any)
			if p["name"] == service {
				_, err = api.Checked("POST", "/api/plugin-switch", map[string]any{"name": service, "groupId": "0", "mode": "enabled", "revision": p["state"].(map[string]any)["revision"]})
				must(err)
			}
		}
	}
	private := fmt.Sprint(qa.Owner)
	v, err = api.Checked("GET", "/api/ai?private="+private, nil)
	must(err)
	_, err = api.Checked("POST", "/api/ai", map[string]any{"revision": v["global"].(map[string]any)["revision"], "fields": map[string]any{"API": srv.URL + "/model", "ModelName": "fixture", "Type": 0}, "keys": map[string]any{"Key": map[string]any{"action": "replace", "value": "synthetic-key"}}})
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
	r, _, err = h.Send(qa.Owner, "fixture-AI-sanity")
	must(err)
	check("private-ai-active", strings.Contains(r, "AI_SANITY") && modelCalls.Load() > 0)
	time.Sleep(100 * time.Millisecond)
	before := modelCalls.Load()
	for _, command := range []string{"抽内置老婆。", "  抽内置老婆！  ", "抽老婆。"} {
		r, _, err = h.Send(qa.Owner, command)
		must(err)
		check("private-punctuation-"+command, strings.Contains(r, "内置角色"))
	}
	_, _, err = h.Send(qa.Owner, "猜老婆。")
	must(err)
	r, _, err = h.Send(qa.Owner, "")
	must(err)
	check("private-punctuation-game-prompt", strings.Contains(r, "请回答"))
	r, _, err = h.Send(qa.Owner, "跳过。")
	must(err)
	check("private-punctuation-game-exit", strings.Contains(r, "已跳过") && strings.Contains(r, "内置角色"))
	time.Sleep(100 * time.Millisecond)
	check("punctuated-commands-no-ai", modelCalls.Load() == before)
	completed = true
	return passed
}
