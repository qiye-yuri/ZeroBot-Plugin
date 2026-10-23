//go:build ignore

// Isolated routing and persistence checks; no real QQ or upstream requests.
package main

import (
	"context"
	ctrl "github.com/FloatTech/zbpctrl"

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
	binary := flag.String("binary-name", "zerobot-status-repeat.exe", "binary under .local/bin")
	boundaries := flag.Bool("repeat-boundaries", false, "also check private, self, image and repeat-cycle boundaries")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(filepath.Join(root, ".local/status-repeat"), 0700))
	out, err := os.MkdirTemp(filepath.Join(root, ".local/status-repeat"), "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
	must(err)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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
		c, stop := context.WithTimeout(ctx, 1800*time.Millisecond)
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
	switchMain("aifalse", "enabled")
	c, stop := context.WithTimeout(ctx, 45*time.Second)
	r, _, e := h.SendWithContext(c, qa.Owner, "自检")
	stop()
	check("status-image", e == nil && h.ImageReceived && !strings.Contains(r, "ERROR"))
	check("member-status-denied", send(23456789, 101, "自检", "member") == "")
	check("member-limit-denied", send(23456789, 101, "设置默认限速为每1秒1次触发", "member") == "")
	for _, cmd := range []string{"设置默认限速为每0秒1次触发", "设置默认限速为每1秒0次触发", "设置默认限速为每9223372036854775807分钟1次触发"} {
		check("invalid-"+cmd, strings.Contains(owner(0, cmd), "ERROR"))
	}
	check("limit-save", strings.Contains(owner(0, "设置默认限速为每2秒1次触发"), "每2秒触发1次"))
	switchMain("breakrepeat", "enabled")
	repeat := func(gid int64, txt string, n int) string {
		r := ""
		for i := 0; i < n; i++ {
			r += owner(gid, txt)
		}
		return r
	}
	check("four-no-reply", repeat(101, "甲", 4) == "")
	check("fifth-short-clean", owner(101, "甲") == "打断复读：甲")
	check("different-message-resets", repeat(101, "测试ABCDE", 3) == "" && owner(101, "换话题") == "" && repeat(101, "测试ABCDE", 4) == "")
	check("fifth-long-reply", strings.HasPrefix(owner(101, "测试ABCDE"), "打断复读："))
	check("groups-isolated", repeat(102, "跨群测试", 4) == "" && owner(103, "跨群测试") == "")
	check("group-102-fifth", strings.HasPrefix(owner(102, "跨群测试"), "打断复读："))
	if *boundaries {
		private := ""
		for i := 0; i < 5; i++ {
			private += owner(0, "私聊复读边界")
		}
		check("private-repeat-ignored", private == "")
		check("self-boundary-four-silent", repeat(104, "自身消息边界", 4) == "")
		check("self-message-ignored", send(87654321, 104, "自身消息边界", "member") == "")
		check("self-does-not-change-count", strings.HasPrefix(owner(104, "自身消息边界"), "打断复读："))
		for index, data := range []map[string]any{
			{"file": "synthetic.png"},
			{"file": "synthetic.png", "url": "https://example.invalid/synthetic.png"},
		} {
			imageReplies := ""
			for i := 0; i < 5; i++ {
				c, stop := context.WithTimeout(ctx, 1800*time.Millisecond)
				r, e := h.SendGroupAs(c, qa.Owner, int64(105+index), "[CQ:image,file=synthetic.png]", "owner", map[string]any{"type": "image", "data": data})
				stop()
				if e != context.DeadlineExceeded {
					must(e)
				}
				imageReplies += r
			}
			check(fmt.Sprintf("image-repeat-ignored-%d", index), imageReplies == "")
		}
		check("text-after-images-four-silent", repeat(105, "甲😀乙", 4) == "")
		r := owner(105, "甲😀乙")
		shuffled := strings.TrimPrefix(r, "打断复读：")
		check("unicode-fifth-preserves-characters", strings.HasPrefix(r, "打断复读：") && len([]rune(shuffled)) == 3 && strings.Count(shuffled, "甲") == 1 && strings.Count(shuffled, "😀") == 1 && strings.Count(shuffled, "乙") == 1)
		check("next-cycle-four-silent", repeat(105, "甲😀乙", 4) == "")
		check("next-cycle-fifth-replies", strings.HasPrefix(owner(105, "甲😀乙"), "打断复读："))
	}
	switchMain("breakrepeat", "disabled")
	check("disabled-repeat", repeat(101, "关闭测试", 5) == "")
	switchMain("aifalse", "disabled")
	check("status-disabled", owner(0, "自检") == "")
	bot.Stop()
	bot, err = h.Start(ctx, "status-restart")
	must(err)
	_, err = api.Auth(h.AdminDir)
	must(err)
	switchMain("aifalse", "enabled")
	bot.Stop()
	m := ctrl.NewManager[any](filepath.Join(out, "data/control/plugins.db"))
	setting := m.NewControl("aifalse", nil)
	check("restart-limit-persisted", setting.GetData(0) == (2|(1<<16)))
	must(m.D.Close())
	completed = true
	return passed
}
