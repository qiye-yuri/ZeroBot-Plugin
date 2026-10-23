//go:build ignore

// Isolated routing and persistence checks; no real QQ or upstream requests.
package main

import (
	"context"
	"database/sql"
	_ "modernc.org/sqlite"

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
	binary := flag.String("binary-name", "zerobot-moderation.exe", "binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(filepath.Join(root, ".local/moderation"), 0700))
	out, err := os.MkdirTemp(filepath.Join(root, ".local/moderation"), "run-")
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
		c, stop := context.WithTimeout(ctx, 5*time.Second)
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
		fmt.Printf("reply: %d chars\n", len([]rune(r)))
		time.Sleep(2100 * time.Millisecond)
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

	switchMain("antiabuse", "enabled")
	switchMain("manager", "enabled")
	check("empty-word-rejected", strings.Contains(owner(101, "添加违禁词"), "不能为空"))
	check("admin-add", strings.Contains(owner(101, "添加违禁词审查E01"), "成功"))
	check("member-add-denied", send(23456789, 101, "添加违禁词无权限", "member") == "")
	r := owner(101, "查看违禁词")
	check("list-image", strings.Contains(r, "本群违禁词") && h.ImageReceived)
	beforeBan := h.ActionCount("set_group_ban")
	beforeDelete := h.ActionCount("delete_msg")
	check("other-group-quiet", send(23456789, 102, "审查E01", "member") == "")
	check("other-group-no-ban", h.ActionCount("set_group_ban") == beforeBan)
	r = send(23456789, 101, "审查E01", "member")
	check("matched-feedback", strings.Contains(r, "检测到违禁词"))
	check("matched-ban-api", h.ActionCount("set_group_ban") == beforeBan+1)
	check("matched-delete-api", h.ActionCount("delete_msg") == beforeDelete+1)
	check("admin-delete", strings.Contains(owner(101, "删除违禁词审查E01"), "成功"))
	check("removed-word-no-match", send(34567890, 101, "审查E01", "member") == "")
	check("removed-word-no-ban", h.ActionCount("set_group_ban") == beforeBan+1)
	check("private-command-ignored", owner(0, "查看违禁词") == "")
	check("set-welcome", strings.Contains(owner(101, "设置欢迎语欢迎E02"), "记住"))
	check("preview-welcome", strings.Contains(owner(101, "测试欢迎语"), "欢迎E02"))
	check("member-welcome-denied", send(34567890, 101, "设置欢迎语非法修改", "member") == "")
	check("welcome-unchanged", strings.Contains(owner(101, "测试欢迎语"), "欢迎E02"))
	check("set-farewell", strings.Contains(owner(101, "设置告别辞再见E03"), "记住"))
	check("preview-farewell", strings.Contains(owner(101, "测试告别辞"), "再见E03"))
	check("welcome-other-group", !strings.Contains(owner(102, "测试欢迎语"), "欢迎E02"))
	check("essence-empty", strings.Contains(owner(101, "/精华列表"), "本群精华列表"))
	check("add-persistent-word", strings.Contains(owner(101, "添加违禁词保存E04"), "成功"))
	ledger, e := sql.Open("sqlite3", filepath.Join(out, "data/anti_abuse/anti_abuse.db"))
	must(e)
	_, e = ledger.Exec("UPDATE __bantime__ SET time=? WHERE id=?", time.Now().Add(-90*time.Second).Unix(), 23456789)
	must(e)
	must(ledger.Close())
	bot.Stop()
	bot, err = h.Start(ctx, "restart")
	must(err)
	api = qa.NewClient(h.Address)
	_, err = api.Auth(h.AdminDir)
	must(err)
	check("welcome-persistent", strings.Contains(owner(101, "测试欢迎语"), "欢迎E02"))
	check("remaining-ban-preserved-after-restart", send(23456789, 101, "查看违禁词", "member") == "")
	check("farewell-persistent", strings.Contains(owner(101, "测试告别辞"), "再见E03"))
	check("word-persistent", strings.Contains(send(45678901, 101, "保存E04", "member"), "检测到违禁词"))
	switchMain("antiabuse", "disabled")
	check("anti-disabled", owner(101, "查看违禁词") == "")
	switchMain("manager", "disabled")
	check("manager-disabled", owner(101, "测试欢迎语") == "")
	completed = true
	return passed
}
