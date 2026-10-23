//go:build ignore

// Isolated routing and persistence checks; no real QQ or upstream requests.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	ctrl "github.com/FloatTech/zbpctrl"
	_ "modernc.org/sqlite"
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
	binary := flag.String("binary-name", "zerobot-bili-merge.exe", "binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(filepath.Join(root, ".local/bili-merge"), 0700))
	out, err := os.MkdirTemp(filepath.Join(root, ".local/bili-merge"), "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
	must(err)
	defer h.Close()
	m := ctrl.NewManager[any](filepath.Join(out, "data/control/plugins.db"))
	legacy := m.NewControl("bilibilipush", nil)
	must(legacy.Disable(101))
	must(legacy.Enable(102))
	retained := m.NewControl("bilibiliparse", nil)
	must(retained.SetData(-qa.Owner, 0x20))
	must(m.D.Close())
	must(os.MkdirAll(filepath.Join(out, "data/bilibilipush"), 0700))
	db, err := sql.Open("sqlite3", filepath.Join(out, "data/bilibilipush/push.db"))
	must(err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE bilibili_push(id integer primary key,bilibili_uid integer,group_id integer,live_disable integer default 0,dynamic_disable integer default 0); INSERT INTO bilibili_push VALUES(1,999001,101,0,1);`)
	must(err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
	switchMain := func(mode string) {
		v, e := api.Checked("GET", "/api/plugins", nil)
		must(e)
		for _, raw := range v["plugins"].([]any) {
			p := raw.(map[string]any)
			if p["name"] == "bilibiliparse" {
				_, e = api.Checked("POST", "/api/plugin-switch", map[string]any{"name": "bilibiliparse", "groupId": "0", "mode": mode, "revision": p["state"].(map[string]any)["revision"]})
				must(e)
				return
			}
		}
		panic("missing main")
	}
	switchMain("enabled")
	r := owner(101, "查看B站功能开关")
	check("legacy-closed-push-preserved", strings.Contains(r, "动态推送：关闭") && strings.Contains(r, "直播推送：关闭") && strings.Contains(r, "链接解析：开启"))
	check("other-group-legacy-enabled", strings.Contains(owner(102, "查看B站功能开关"), "动态推送：开启"))
	check("member-cannot-enable", send(23456789, 101, "开启B站动态推送", "member") == "")
	check("enable-dynamic", strings.Contains(owner(101, "开启B站动态推送"), "已开启"))
	r = owner(101, "查看B站功能开关")
	check("independent-live-gate", strings.Contains(r, "动态推送：开启") && strings.Contains(r, "直播推送：关闭"))
	check("disable-parse", strings.Contains(owner(101, "关闭B站链接解析"), "已关闭"))
	check("disabled-parse-no-network", owner(101, "https://www.bilibili.com/video/BV1234567890") == "")
	check("group-isolation", strings.Contains(owner(102, "查看B站功能开关"), "链接解析：开启"))
	check("private-cookie-only", owner(101, "设置b站cookie synthetic=group-denied") == "")
	check("member-cookie-denied", send(23456789, 0, "设置b站cookie synthetic=member-denied", "member") == "")
	r = owner(0, "设置b站cookie synthetic=private-test")
	check("cookie-saved-no-echo", strings.Contains(r, "已保存") && !strings.Contains(r, "private-test"))
	raw, err := os.ReadFile(filepath.Join(out, "data/Bilibili/config.json"))
	must(err)
	check("legacy-cookie-path", strings.Contains(string(raw), "synthetic=private-test"))
	check("private-summary-on", strings.Contains(owner(0, "开启视频总结"), "已开启"))
	check("upload-command-removed", owner(0, "开启视频上传") == "")
	check("upload-close-command-removed", owner(0, "关闭视频上传") == "")
	check("private-summary-off", strings.Contains(owner(0, "关闭视频总结"), "已关闭"))
	// Read saved settings through a separate control manager after the process exits.
	switchMain("disabled")
	for _, cmd := range []string{"设置b站cookie synthetic=blocked", ">user info 999001", "b站推送列表", "拉取b站推送", "查看B站功能开关"} {
		check("unified-off-"+cmd, owner(0, cmd) == "")
	}
	raw, err = os.ReadFile(filepath.Join(out, "data/Bilibili/config.json"))
	must(err)
	check("disabled-cookie-unchanged", !strings.Contains(string(raw), "blocked"))
	switchMain("enabled")
	bot.Stop()
	m = ctrl.NewManager[any](filepath.Join(out, "data/control/plugins.db"))
	main := m.NewControl("bilibiliparse", nil)
	check("retired-upload-bit-preserved-inert", main.GetData(-qa.Owner)&0x30 == 0x20)
	must(m.D.Close())
	bot, err = h.Start(ctx, "bili-restart")
	must(err)
	r = owner(101, "查看B站功能开关")
	check("restart-gates-persist", strings.Contains(r, "链接解析：关闭") && strings.Contains(r, "动态推送：开启") && strings.Contains(r, "直播推送：关闭"))
	var count int
	must(db.QueryRow("SELECT count(*) FROM bilibili_push WHERE bilibili_uid=999001 AND group_id=101 AND live_disable=0 AND dynamic_disable=1").Scan(&count))
	check("legacy-subscription-preserved", count == 1)
	completed = true
	return passed
}
