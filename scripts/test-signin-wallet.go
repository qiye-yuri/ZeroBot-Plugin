//go:build ignore

// Isolated routing and persistence checks; no real QQ or upstream requests.
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
	atomicity := flag.Bool("transfer-atomicity", false, "test rollback on recipient write failure")
	boundaries := flag.Bool("wallet-boundaries", false, "test rejected transfers leave both synthetic balances unchanged")
	binary := flag.String("binary-name", "zerobot-signin-wallet.exe", "binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(filepath.Join(root, ".local/signin-wallet"), 0700))
	out, err := os.MkdirTemp(filepath.Join(root, ".local/signin-wallet"), "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
	must(err)
	defer h.Close()
	must(os.MkdirAll(filepath.Join(out, "data/wallet"), 0700))
	must(os.WriteFile(filepath.Join(out, "data/wallet/coin_name.txt"), []byte("测试币"), 0600))
	db, err := sql.Open("sqlite3", filepath.Join(out, "data/wallet/wallet.db"))
	must(err)
	defer db.Close()
	_, err = db.Exec("CREATE TABLE storage(uid integer primary key,money integer); INSERT INTO storage VALUES(12345678,100),(23456789,0)")
	must(err)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
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
		// The shared user bucket refills one token per 10 seconds (burst five).
		if strings.HasPrefix(cmd, "签到") || strings.HasPrefix(cmd, "设置签到预设") {
			time.Sleep(10100 * time.Millisecond)
		} else {
			time.Sleep(2100 * time.Millisecond)
		}
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
			if p["name"] == "score" {
				_, e = api.Checked("POST", "/api/plugin-switch", map[string]any{"name": "score", "groupId": "0", "mode": mode, "revision": p["state"].(map[string]any)["revision"]})
				must(e)
				return
			}
		}
		panic("missing main")
	}
	switchMain("enabled")
	check("legacy-balance-and-coin", strings.Contains(owner(0, "查看我的钱包"), "钱包有100测试币"))
	check("admin-credit", strings.Contains(owner(0, "管理钱包余额+20"), "修改成功"))
	check("balance-credit", strings.Contains(owner(0, "查看我的钱包"), "钱包有120测试币"))
	check("invalid-amount", strings.Contains(owner(0, "管理钱包余额abc"), "金额异常"))
	check("insufficient-admin-debit", strings.Contains(owner(0, "管理钱包余额-1000"), "余额不足"))
	check("member-admin-denied", send(23456789, 0, "管理钱包余额+50", "member") == "")
	transfer := func(amount string) string {
		c, stop := context.WithTimeout(ctx, 3*time.Second)
		defer stop()
		cmd := "钱包转账" + amount
		segments := []map[string]any{{"type": "text", "data": map[string]string{"text": cmd}}, {"type": "at", "data": map[string]string{"qq": "23456789"}}}
		r, e := h.SendGroupAs(c, qa.Owner, 101, cmd+"[CQ:at,qq=23456789]", "owner", segments...)
		must(e)
		return r
	}
	if *atomicity {
		_, err = db.Exec("CREATE TRIGGER fail_credit BEFORE INSERT ON storage WHEN NEW.uid=23456789 BEGIN SELECT RAISE(ABORT,'synthetic credit failure'); END;")
		must(err)
		time.Sleep(2100 * time.Millisecond)
		reply := transfer("10")
		check("credit-failure-reported", strings.Contains(reply, "转账失败"))
		var a, b int
		must(db.QueryRow("SELECT money FROM storage WHERE uid=12345678").Scan(&a))
		must(db.QueryRow("SELECT money FROM storage WHERE uid=23456789").Scan(&b))
		check("failed-credit-rolls-back-both", a == 120 && b == 0)
		_, err = db.Exec("DROP TRIGGER fail_credit")
		must(err)
		time.Sleep(2100 * time.Millisecond)
		check("recovery-transfer-success", strings.Contains(transfer("10"), "转账成功"))
		must(db.QueryRow("SELECT money FROM storage WHERE uid=12345678").Scan(&a))
		must(db.QueryRow("SELECT money FROM storage WHERE uid=23456789").Scan(&b))
		check("recovery-balances-conserved", a == 110 && b == 10)
		bot.Stop()
		bot, err = h.Start(ctx, "atomic-restart")
		must(err)
		check("restart-keeps-atomic-balance", strings.Contains(owner(0, "查看我的钱包"), "钱包有110测试币"))
		completed = true
		return passed
	}
	if *boundaries {
		for _, tc := range []struct{ amount, want string }{
			{"0", "输入额异常"}, {"-1", "输入额异常"},
			{"9999999999999999999999999999999", "输入额异常"},
			{"1000", "余额不足"},
		} {
			time.Sleep(2100 * time.Millisecond)
			check("reject-"+tc.amount, strings.Contains(transfer(tc.amount), tc.want))
			var a, b int
			must(db.QueryRow("SELECT money FROM storage WHERE uid=12345678").Scan(&a))
			must(db.QueryRow("SELECT money FROM storage WHERE uid=23456789").Scan(&b))
			check("unchanged-"+tc.amount, a == 120 && b == 0)
		}
		check("missing-recipient", strings.Contains(owner(101, "钱包转账1"), "获取被转方信息失败"))
		check("private-denied", owner(0, "钱包转账1") == "")
		bot.Stop()
		bot, err = h.Start(ctx, "restart")
		must(err)
		check("restart-no-debit", strings.Contains(owner(0, "查看我的钱包"), "钱包有120测试币"))
		var b int
		must(db.QueryRow("SELECT money FROM storage WHERE uid=23456789").Scan(&b))
		check("restart-no-credit", b == 0)
		completed = true
		return passed
	}
	check("group-transfer", strings.Contains(transfer("10"), "转账成功"))
	// Inspect the synthetic ledger directly: immediate repeated queries can hit the shared rate limit.
	var senderBalance, recipientBalance int
	must(db.QueryRow("SELECT money FROM storage WHERE uid=12345678").Scan(&senderBalance))
	must(db.QueryRow("SELECT money FROM storage WHERE uid=23456789").Scan(&recipientBalance))
	check("sender-debited", senderBalance == 110)
	check("recipient-credited", recipientBalance == 10)
	check("transfer-insufficient", strings.Contains(transfer("1000"), "余额不足"))
	check("transfer-zero", strings.Contains(transfer("0"), "输入额异常"))
	check("private-transfer-denied", owner(0, "钱包转账10") == "")
	check("signin-invalid-style", strings.Contains(owner(0, "签到99"), "未找到签到设定"))
	for _, key := range []string{"256", "257", "259", "65536"} {
		check("signin-no-wrap-"+key, strings.Contains(owner(0, "签到"+key), "未找到签到设定"))
		check("preset-no-wrap-"+key, strings.Contains(owner(0, "设置签到预设"+key), "未找到签到设定"))
	}
	for _, key := range []string{"0", "1", "2", "3"} {
		check("preset-valid-"+key, strings.Contains(owner(0, "设置签到预设"+key), "设置成功"))
	}
	check("preset-member-denied", send(23456789, 0, "设置签到预设1", "member") == "")

	switchMain("disabled")
	check("shared-gate-wallet", owner(0, "查看我的钱包") == "")
	check("shared-gate-signin", owner(0, "签到99") == "")
	switchMain("enabled")
	check("restore-balance", strings.Contains(owner(0, "查看我的钱包"), "钱包有110测试币"))
	bot.Stop()
	bot, err = h.Start(ctx, "restart")
	must(err)
	check("restart-preserves-wallet", strings.Contains(owner(0, "查看我的钱包"), "钱包有110测试币"))
	completed = true
	return passed
}
