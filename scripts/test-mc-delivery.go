//go:build ignore

package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/binary"
	"flag"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	_ "github.com/FloatTech/sqlite"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	availability := flag.Bool("availability", false, "test offline/recovery lifecycle")
	flag.Parse()
	root, e := os.Getwd()
	must(e)
	parent := filepath.Join(root, ".local/mc-delivery")
	must(os.MkdirAll(parent, 0700))
	out, e := os.MkdirTemp(parent, "run-")
	must(e)
	fmt.Println("ARTIFACTS=" + out)
	h, e := qa.New(root, filepath.Join(root, ".local/bin/zerobot-mc-delivery.exe"), out)
	must(e)
	defer h.Close()
	var version atomic.Int64
	version.Store(1)
	addr, closeServer := mockServer(&version)
	defer closeServer()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	bot, e := h.Start(ctx, "mc")
	must(e)
	defer bot.Stop()
	checks := []map[string]any{}
	passed, complete := true, false
	defer func() {
		must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed && complete, "checks": checks, "realQQMessagesSent": false}))
	}()
	check := func(n string, ok bool) {
		checks = append(checks, map[string]any{"name": n, "passed": ok})
		passed = passed && ok
		fmt.Println(n, ok)
	}
	send := func(cmd string) string {
		c, stop := context.WithTimeout(ctx, 3*time.Second)
		defer stop()
		r, _, e := h.SendWithContext(c, qa.Owner, cmd)
		if e != context.DeadlineExceeded {
			must(e)
		}
		fmt.Printf("reply %s: %q\n", cmd, r)
		time.Sleep(200 * time.Millisecond)
		return r
	}
	check("add", strings.Contains(send("mc服务器添加订阅 "+addr), "订阅添加成功"))
	c, stop := context.WithTimeout(ctx, 3*time.Second)
	r, e := h.AwaitReply(c)
	stop()
	must(e)
	check("initial-status", strings.Contains(r, "fixture-1"))
	time.Sleep(200 * time.Millisecond)
	db, e := sql.Open("sqlite3", filepath.Join(out, "data/minecraftobserver/minecraft_observer"))
	must(e)
	defer db.Close()
	pending := func() int { var n int; must(db.QueryRow("SELECT COUNT(*) FROM pending_notices").Scan(&n)); return n }
	if *availability {
		pull := func() string { return send("mc服务器订阅拉取") }
		version.Store(-1)
		check("transient-first-silent", pull() == "")
		version.Store(1)
		check("transient-recovery-silent", pull() == "")
		version.Store(-1)
		check("offline-first-silent", pull() == "")
		check("offline-second-silent", pull() == "")
		check("offline-third-notifies", strings.Contains(pull(), "[Minecraft服务器状态变更通知]"))
		var delay int64
		must(db.QueryRow("SELECT ping_delay FROM server_statuses WHERE server_addr = ?", addr).Scan(&delay))
		check("offline-baseline-persisted", delay == -1)
		for i := 0; i < 4; i++ {
			check(fmt.Sprintf("offline-no-duplicate-%d", i+1), pull() == "")
		}
		bot.Stop()
		bot, e = h.Start(ctx, "mc-restart")
		must(e)
		defer bot.Stop()
		for i := 0; i < 3; i++ {
			check(fmt.Sprintf("offline-after-restart-silent-%d", i+1), pull() == "")
		}
		version.Store(1)
		r := pull()
		check("recovery-notifies", strings.Contains(r, "[Minecraft服务器状态变更通知]") && strings.Contains(r, "fixture-1"))
		must(db.QueryRow("SELECT ping_delay FROM server_statuses WHERE server_addr = ?", addr).Scan(&delay))
		check("recovery-baseline-persisted", delay >= 0)
		check("recovery-no-duplicate", pull() == "")
		check("delivery-queue-empty", pending() == 0)
		bot.Stop()
		for _, label := range []string{"mc", "mc-restart"} {
			logs, e := os.ReadFile(filepath.Join(out, label+"-stderr.log"))
			must(e)
			check(label+"-no-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
		}
		complete = true
		return passed
	}
	check("unchanged-silent", send("mc服务器订阅拉取") == "")
	version.Store(2)
	h.FailAction("send_private_msg")
	check("changed-attempt", strings.Contains(send("mc服务器订阅拉取"), "fixture-2"))
	time.Sleep(100 * time.Millisecond)
	check("failure-persisted", pending() == 1)
	bot.Stop()
	bot, e = h.Start(ctx, "mc-restart")
	must(e)
	defer bot.Stop()
	check("pending-survives-restart", pending() == 1)
	h.FailAction("")
	check("retry-no-new-change", strings.Contains(send("mc服务器订阅拉取"), "fixture-2"))
	time.Sleep(100 * time.Millisecond)
	check("receipt-clears-queue", pending() == 0)
	check("no-duplicate-after-success", send("mc服务器订阅拉取") == "")
	version.Store(3)
	h.FailAction("send_private_msg")
	check("second-change-attempt", strings.Contains(send("mc服务器订阅拉取"), "fixture-3"))
	time.Sleep(100 * time.Millisecond)
	check("second-change-pending", pending() == 1)
	h.FailAction("")
	check("cancel", strings.Contains(send("mc服务器取消订阅 "+addr), "取消订阅成功"))
	check("cancelled-no-delivery", send("mc服务器订阅拉取") == "")
	check("cancelled-queue-cleared", pending() == 0)
	bot.Stop()
	for _, label := range []string{"mc", "mc-restart"} {
		logs, e := os.ReadFile(filepath.Join(out, label+"-stderr.log"))
		must(e)
		check(label+"-no-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
	}
	complete = true
	return passed
}
func mockServer(version *atomic.Int64) (string, func()) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				if version.Load() < 0 {
					return
				}
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				r := bufio.NewReader(conn)
				read := func() ([]byte, error) {
					n, e := binary.ReadUvarint(r)
					if e != nil {
						return nil, e
					}
					if n > 4096 {
						return nil, io.ErrShortBuffer
					}
					b := make([]byte, int(n))
					_, e = io.ReadFull(r, b)
					return b, e
				}
				write := func(b []byte) error {
					var prefix [10]byte
					n := binary.PutUvarint(prefix[:], uint64(len(b)))
					_, e := conn.Write(append(prefix[:n], b...))
					return e
				}
				if _, err := read(); err != nil {
					return
				} // handshake
				if _, err := read(); err != nil {
					return
				} // status request
				status := []byte(fmt.Sprintf(`{"version":{"name":"fixture-%d","protocol":763},"players":{"max":20,"online":2},"description":{"text":"本地测试服务器"}}`, version.Load()))
				var size [10]byte
				n := binary.PutUvarint(size[:], uint64(len(status)))
				packet := append([]byte{0}, size[:n]...)
				packet = append(packet, status...)
				if write(packet) != nil {
					return
				}
				ping, err := read()
				if err == nil {
					_ = write(ping)
				}
			}()
		}
	}()
	return l.Addr().String(), func() { l.Close() }
}
