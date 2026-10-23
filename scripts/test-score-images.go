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
	must(os.MkdirAll(".local/score-images", 0700))
	out, e := os.MkdirTemp(filepath.Join(root, ".local/score-images"), "run-")
	must(e)
	fmt.Println("ARTIFACTS=" + out)
	h, e := qa.New(root, filepath.Join(root, ".local/bin/zerobot-score-images.exe"), out)
	must(e)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	bot, e := h.Start(ctx, "images")
	must(e)
	defer bot.Stop()
	api := qa.NewClient(h.Address)
	_, e = api.Auth(h.AdminDir)
	must(e)
	v, e := api.Checked("GET", "/api/plugins", nil)
	must(e)
	for _, raw := range v["plugins"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "score" {
			_, e = api.Checked("POST", "/api/plugin-switch", map[string]any{"name": "score", "groupId": "0", "mode": "enabled", "revision": p["state"].(map[string]any)["revision"]})
			must(e)
		}
	}
	score, e := sql.Open("sqlite3", filepath.Join(out, "data/score/score.db"))
	must(e)
	defer score.Close()
	wallet, e := sql.Open("sqlite3", filepath.Join(out, "data/wallet/wallet.db"))
	must(e)
	defer wallet.Close()
	_, e = wallet.Exec("CREATE TABLE IF NOT EXISTS storage(uid integer primary key,money integer)")
	must(e)
	_, e = wallet.Exec("INSERT OR REPLACE INTO storage VALUES(?,100)", qa.Owner)
	must(e)
	_, e = score.Exec("INSERT OR REPLACE INTO score(uid,score) VALUES(?,3)", qa.Owner)
	must(e)
	_, e = score.Exec("INSERT OR REPLACE INTO sign_in(uid,count,updated_at) VALUES(?,1,?)", qa.Owner, time.Now())
	must(e)
	passed := true
	checks := []map[string]any{}
	check := func(n string, ok bool) {
		passed = passed && ok
		checks = append(checks, map[string]any{"name": n, "passed": ok})
		fmt.Println(n, ok)
	}
	send := func(uid int64, text string) string {
		c, stop := context.WithTimeout(ctx, 20*time.Second)
		defer stop()
		r, _, err := h.SendWithContext(c, uid, text)
		must(err)
		return r
	}
	check("not-signed-denied", strings.Contains(send(23456789, "补发签到图"), "还没有签到"))
	for i := 0; i < 2; i++ {
		if i > 0 {
			time.Sleep(11 * time.Second)
		}
		check(fmt.Sprintf("repair-notice-%d", i), strings.Contains(send(qa.Owner, "补发签到图"), "不增加"))
		c, stop := context.WithTimeout(ctx, 25*time.Second)
		_, e = h.AwaitReply(c)
		stop()
		check(fmt.Sprintf("repair-image-%d", i), e == nil && h.ImageReceived)
		var money, level, count int
		must(wallet.QueryRow("SELECT money FROM storage WHERE uid=?", qa.Owner).Scan(&money))
		must(score.QueryRow("SELECT score FROM score WHERE uid=?", qa.Owner).Scan(&level))
		must(score.QueryRow("SELECT count FROM sign_in WHERE uid=?", qa.Owner).Scan(&count))
		check(fmt.Sprintf("ledger-unchanged-%d", i), money == 100 && level == 3 && count == 1)
	}
	c, stop := context.WithTimeout(ctx, 10*time.Second)
	_, e = h.SendGroupAs(c, qa.Owner, 101, "获得签到背景", "owner")
	stop()
	check("background-accessible", e == nil && h.ImageReceived)
	must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed, "checks": checks, "realQQMessagesSent": false}))
	return passed
}
