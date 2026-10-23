//go:build ignore

// Destructive reset commands are exercised only on synthetic isolated data.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func must(e error) {
	if e != nil {
		panic(e)
	}
}
func main() {
	if !run() {
		os.Exit(1)
	}
}
func run() bool {
	binary := flag.String("binary-name", "zerobot-qqwife-reset.exe", "binary under .local/bin")
	settings := flag.Bool("settings-boundaries", false, "also verify settings permissions, group isolation and restart persistence")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, e := os.Getwd()
	must(e)
	parent := filepath.Join(root, ".local/qqwife-reset")
	must(os.MkdirAll(parent, 0700))
	out, e := os.MkdirTemp(parent, "run-")
	must(e)
	fmt.Println("ARTIFACTS=" + out)
	h, e := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
	must(e)
	defer h.Close()
	folder := filepath.Join(out, "data/qqwife")
	must(os.MkdirAll(folder, 0700))
	db, e := sql.Open("sqlite3", filepath.Join(folder, "结婚登记表.db"))
	must(e)
	defer db.Close()
	_, e = db.Exec(`CREATE TABLE updateinfo(GID integer PRIMARY KEY,Updatetime text,CanMatch integer,CanNtr integer,CDtime real);
 INSERT INTO updateinfo VALUES(101,'synthetic',1,0,12),(102,'synthetic',0,1,6);
 CREATE TABLE cdsheet(Time integer,GroupID integer,UserID integer,ModeID text);
 INSERT INTO cdsheet VALUES(1,101,12345678,'fixture'),(2,102,23456789,'fixture');
 CREATE TABLE favorability(Userinfo text,Favor integer); INSERT INTO favorability VALUES('fixture',27);
 CREATE TABLE group101(User integer,Target integer,Username text,Targetname text,Updatetime text);
 INSERT INTO group101 VALUES(12345678,23456789,'A','B','synthetic');
 CREATE TABLE group102(User integer,Target integer,Username text,Targetname text,Updatetime text);
 INSERT INTO group102 VALUES(34567890,45678901,'C','D','synthetic');`)
	must(e)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	bot, e := h.Start(ctx, "reset")
	must(e)
	defer func() { bot.Stop() }()
	api := qa.NewClient(h.Address)
	_, e = api.Auth(h.AdminDir)
	must(e)
	catalog, e := api.Checked("GET", "/api/plugins", nil)
	must(e)
	for _, raw := range catalog["plugins"].([]any) {
		p := raw.(map[string]any)
		mode := ""
		if p["name"] == "aichat" {
			mode = "disabled"
		}
		if p["name"] == "qqwife" {
			mode = "enabled"
		}
		if mode != "" {
			_, e = api.Checked("POST", "/api/plugin-switch", map[string]any{"name": p["name"], "groupId": "0", "mode": mode, "revision": p["state"].(map[string]any)["revision"]})
			must(e)
		}
	}
	passed, complete := true, false
	checks := []map[string]any{}
	defer func() {
		must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed && complete, "checks": checks, "isolated": true, "realQQMessagesSent": false}))
	}()
	check := func(name string, ok bool) {
		passed = passed && ok
		checks = append(checks, map[string]any{"name": name, "passed": ok})
		fmt.Println(name, ok)
	}
	number := func(query string) int { var n int; must(db.QueryRow(query).Scan(&n)); return n }
	exists := func(table string) bool {
		var n int
		must(db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&n))
		return n == 1
	}
	send := func(uid, gid int64, cmd, role string) string {
		time.Sleep(2100 * time.Millisecond)
		c, done := context.WithTimeout(ctx, 2*time.Second)
		defer done()
		var r string
		var err error
		if gid == 0 {
			r, _, err = h.SendWithContext(c, uid, cmd)
		} else {
			r, err = h.SendGroupAs(c, uid, gid, cmd, role)
		}
		if err != context.DeadlineExceeded {
			must(err)
		}
		return r
	}
	if *settings {
		state := func(gid int64) string {
			var match, ntr int
			var cd float64
			must(db.QueryRow("SELECT CanMatch,CanNtr,CDtime FROM updateinfo WHERE GID=?", gid).Scan(&match, &ntr, &cd))
			return fmt.Sprintf("%d/%d/%g", match, ntr, cd)
		}
		for _, cmd := range []string{"禁止自由恋爱", "允许牛头人", "设置CD为3小时"} {
			check("member-settings-denied-"+cmd, send(23456789, 101, cmd, "member") == "")
		}
		check("denied-settings-unchanged", state(101) == "1/0/12" && state(102) == "0/1/6")
		check("private-settings-ignored", send(qa.Owner, 0, "设置CD为3小时", "owner") == "" && number("SELECT count(*) FROM updateinfo WHERE GID=0") == 0)
		check("admin-disable-match", strings.Contains(send(34567890, 101, "禁止自由恋爱", "admin"), "设置成功") && state(101) == "0/0/12")
		check("admin-enable-ntr", strings.Contains(send(34567890, 101, "允许牛头人", "admin"), "设置成功") && state(101) == "0/1/12")
		check("admin-update-cooldown", strings.Contains(send(34567890, 101, "设置CD为3小时", "admin"), "设置成功") && state(101) == "0/1/3")
		check("other-group-settings-unchanged", state(102) == "0/1/6")
		check("settings-preserve-game-records", number("SELECT count(*) FROM group101") == 1 && number("SELECT count(*) FROM group102") == 1 && number("SELECT count(*) FROM cdsheet") == 2 && number("SELECT Favor FROM favorability") == 27)
		bot.Stop()
		bot, e = h.Start(ctx, "settings-restart")
		must(e)
		check("restart-settings-retained", state(101) == "0/1/3" && state(102) == "0/1/6")
		check("restore-match", strings.Contains(send(qa.Owner, 101, "允许自由恋爱", "owner"), "设置成功") && state(101) == "1/1/3")
		check("restore-ntr", strings.Contains(send(qa.Owner, 101, "禁止牛头人", "owner"), "设置成功") && state(101) == "1/0/3")
		check("restore-cooldown", strings.Contains(send(qa.Owner, 101, "设置CD为12小时", "owner"), "设置成功") && state(101) == "1/0/12")
		check("settings-no-duplicate-rows", number("SELECT count(*) FROM updateinfo") == 2)
	}
	check("member-denied", send(23456789, 101, "重置102花名册", "member") == "")
	check("admin-denied", send(34567890, 101, "重置所有花名册", "admin") == "")
	check("private-default-rejected", strings.Contains(send(qa.Owner, 0, "重置花名册", "owner"), "只能在群组"))
	check("zero-rejected", strings.Contains(send(qa.Owner, 0, "重置0花名册", "owner"), "正确的群号"))
	check("overflow-rejected", strings.Contains(send(qa.Owner, 0, "重置999999999999999999999999花名册", "owner"), "正确的群号"))
	check("rejections-preserve-records", number("SELECT count(*) FROM group101") == 1 && number("SELECT count(*) FROM group102") == 1 && number("SELECT count(*) FROM cdsheet") == 2)
	check("explicit-target-reset", strings.Contains(send(qa.Owner, 0, "重置102花名册", "owner"), "重置成功"))
	check("target-only-removed", !exists("group102") && number("SELECT count(*) FROM group101") == 1)
	check("target-cooldown-only-removed", number("SELECT count(*) FROM cdsheet WHERE GroupID=102") == 0 && number("SELECT count(*) FROM cdsheet WHERE GroupID=101") == 1)
	check("target-settings-retained", number("SELECT count(*) FROM updateinfo") == 2)
	check("target-favor-retained", number("SELECT Favor FROM favorability") == 27)
	check("current-group-reset", strings.Contains(send(qa.Owner, 101, "重置本群花名册", "owner"), "重置成功"))
	check("current-group-removed", !exists("group101") && number("SELECT count(*) FROM cdsheet") == 0)
	check("all-reset", strings.Contains(send(qa.Owner, 0, "重置所有花名册", "owner"), "重置成功"))
	check("all-settings-removed", !exists("updateinfo") && !exists("cdsheet"))
	check("all-favor-retained", number("SELECT Favor FROM favorability") == 27)
	bot.Stop()
	logs, e := os.ReadFile(filepath.Join(out, "reset-stderr.log"))
	must(e)
	check("no-handler-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
	if *settings {
		logs, e = os.ReadFile(filepath.Join(out, "settings-restart-stderr.log"))
		must(e)
		check("restart-no-handler-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
	}
	complete = true
	return passed
}
