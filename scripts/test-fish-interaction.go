//go:build ignore

package main

import (
	"context"
	"database/sql"
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
	root, e := os.Getwd()
	must(e)
	must(os.MkdirAll(".local/fish-interaction", 0700))
	out, e := os.MkdirTemp(filepath.Join(root, ".local/fish-interaction"), "run-")
	must(e)
	fmt.Println("ARTIFACTS=" + out)
	h, e := qa.New(root, filepath.Join(root, ".local/bin/zerobot-fish-interaction-verified.exe"), out)
	must(e)
	defer h.Close()
	db, e := sql.Open("sqlite3", filepath.Join(out, "data/McFish/fishdata.db"))
	must(e)
	defer db.Close()
	_, e = db.Exec(`CREATE TABLE "12345678Pack"(Duration integer primary key,Name text,Number integer,Other text,Type text);INSERT INTO "12345678Pack" VALUES(1,'木竿',1,'30/0/0/0','pole'),(2,'木竿',1,'40/0/0/0','pole'),(3,'木竿',1,'50/0/0/0','pole'),(4,'鳕鱼',2,'','fish'),(5,'唱片',1,'','treasure')`)
	must(e)
	must(os.MkdirAll(filepath.Join(out, "data/wallet"), 0700))
	wallet, e := sql.Open("sqlite3", filepath.Join(out, "data/wallet/wallet.db"))
	must(e)
	defer wallet.Close()
	_, e = wallet.Exec("CREATE TABLE storage(uid integer primary key,money integer);INSERT INTO storage VALUES(12345678,10000)")
	must(e)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	bot, e := h.Start(ctx, "fish")
	must(e)
	defer bot.Stop()
	api := qa.NewClient(h.Address)
	_, e = api.Auth(h.AdminDir)
	must(e)
	toggle := func(name, mode string) {
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
	passed := true
	checks := []map[string]any{}
	defer func() {
		must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed, "checks": checks, "realQQMessagesSent": false}))
	}()
	check := func(n string, ok bool) {
		passed = passed && ok
		checks = append(checks, map[string]any{"name": n, "passed": ok})
		fmt.Println(n, ok)
	}
	send := func(uid, gid int64, text string) string {
		c, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		var r string
		var err error
		if gid == 0 {
			r, _, err = h.SendWithContext(c, uid, text)
		} else {
			r, err = h.SendGroupAs(c, uid, gid, text, "owner")
		}
		if err != context.DeadlineExceeded {
			must(err)
		}
		time.Sleep(300 * time.Millisecond)
		return r
	}
	owner := func(text string) string { return send(qa.Owner, 0, text) }
	toggle("mcfish", "enabled")
	check("equip-list", strings.Contains(owner("装备木竿"), "找到以下鱼竿"))
	check("other-session-ignored", send(qa.Owner, 101, "1") == "")
	check("other-user-ignored", send(23456789, 0, "1") == "")
	toggle("aichat", "enabled")
	check("invalid-index", strings.Contains(owner("999"), "正确的序号"))
	r := owner("1")
	check("equip-selection-not-stolen", strings.Contains(r, "装备") && !strings.Contains(r, "ERROR"))
	var remaining int
	must(db.QueryRow(`SELECT SUM(Number) FROM "12345678Pack" WHERE Name='木竿'`).Scan(&remaining))
	check("one-pole-equipped", remaining == 2)
	check("repair-list", strings.Contains(owner("修复鱼竿"), "找到以下鱼竿"))
	check("repair-cancel", strings.Contains(owner("取消"), "取消修复"))
	check("sale-confirm", strings.Contains(owner("出售鳕鱼"), "是否接受"))
	check("second-sale-phase", strings.Contains(owner("是"), "是否使用唱片"))
	r = owner("否")
	check("second-phase-not-stolen", r != "" && !strings.Contains(r, "ERROR") && !strings.Contains(r, "等待超时"))
	var fish, record, money int
	must(db.QueryRow(`SELECT Number FROM "12345678Pack" WHERE Name='鳕鱼'`).Scan(&fish))
	must(db.QueryRow(`SELECT Number FROM "12345678Pack" WHERE Name='唱片'`).Scan(&record))
	must(wallet.QueryRow("SELECT money FROM storage WHERE uid=12345678").Scan(&money))
	check("sale-updated-once", fish == 1 && record == 1 && money > 10000)
	time.Sleep(4 * time.Second)
	_, e = db.Exec("UPDATE buff SET Buff0=19,Buff1=2 WHERE ID=?", qa.Owner)
	must(e)
	check("purchase-confirm", strings.Contains(owner("购买初始木竿"), "确定花费"))
	check("purchase-cancel", strings.Contains(owner("否"), "已取消购买"))
	var after int
	must(wallet.QueryRow("SELECT money FROM storage WHERE uid=12345678").Scan(&after))
	check("cancel-balance-unchanged", after == money)
	var coupon, buyTimes int
	must(db.QueryRow("SELECT Buff0,Buff1 FROM buff WHERE ID=?", qa.Owner).Scan(&buyTimes, &coupon))
	check("cancel-benefits-unchanged", buyTimes == 19 && coupon == 2)
	time.Sleep(4 * time.Second)
	check("purchase-again", strings.Contains(owner("购买初始木竿"), "确定花费"))
	r = owner("是")
	check("purchase-success", r != "" && !strings.Contains(r, "ERROR"))
	must(db.QueryRow("SELECT Buff0,Buff1 FROM buff WHERE ID=?", qa.Owner).Scan(&buyTimes, &coupon))
	check("success-uses-one-coupon", buyTimes == 19 && coupon == 1)
	must(db.QueryRow(`SELECT SUM(Number) FROM "12345678Pack" WHERE Name='木竿'`).Scan(&remaining))
	must(wallet.QueryRow("SELECT money FROM storage WHERE uid=12345678").Scan(&after))
	check("purchase-item-and-debit-once", remaining == 3 && after < money)

	toggle("aichat", "disabled")
	fmt.Println("observing-past-120-second-deadlines")
	c, stop := context.WithTimeout(ctx, 125*time.Second)
	late, err := h.AwaitReply(c)
	stop()
	check("no-late-timeout-or-ai-reply", late == "" && err == context.DeadlineExceeded)

	bot.Stop()
	logData, err := os.ReadFile(filepath.Join(out, "fish-stderr.log"))
	must(err)
	check("no-handler-panic", !strings.Contains(string(logData), "execute handler err") && !strings.Contains(string(logData), "panic:"))
	return passed
}
