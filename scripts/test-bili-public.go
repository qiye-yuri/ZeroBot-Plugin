//go:build ignore

// Synthetic QQ, real public upstream endpoints; no saved account credentials.
package main

import (
	"context"
	"flag"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	"os"
	"path/filepath"
	"regexp"
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
	mobile := flag.Bool("mobile", false, "test mobile and escaped share links; requires article")
	article := flag.String("article", "", "when set, test legacy article and unavailable content")
	opus := flag.String("opus", "", "public opus ID")
	live := flag.String("live", "", "public room ID")
	flag.Parse()
	digits := regexp.MustCompile(`^\d+$`)
	if !digits.MatchString(*opus) || !digits.MatchString(*live) {
		panic("numeric IDs required")
	}
	root, e := os.Getwd()
	must(e)
	parent := filepath.Join(root, ".local/bili-opus")
	must(os.MkdirAll(parent, 0700))
	out, e := os.MkdirTemp(parent, "run-")
	must(e)
	fmt.Println("ARTIFACTS=" + out)
	h, e := qa.New(root, filepath.Join(root, ".local/bin/zerobot-bili-opus.exe"), out)
	must(e)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	bot, e := h.Start(ctx, "public")
	must(e)
	defer bot.Stop()
	checks := []map[string]any{}
	passed, complete := true, false
	defer func() {
		must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed && complete, "checks": checks, "realQQMessagesSent": false, "realPublicAPIs": true, "cookieUsed": false}))
	}()
	check := func(name string, ok bool) {
		checks = append(checks, map[string]any{"name": name, "passed": ok})
		passed = passed && ok
		fmt.Println(name, ok)
	}
	_, e = os.Stat(filepath.Join(out, "data/Bilibili/config.json"))
	check("no-cookie-before", os.IsNotExist(e))
	type contentCase struct {
		name, url, marker string
		failure           bool
	}
	cases := []contentCase{
		{"opus", "https://www.bilibili.com/opus/" + *opus, "作者：", false},
		{"legacy-dynamic", "https://t.bilibili.com/" + *opus, "作者：", false},
		{"live", "https://live.bilibili.com/" + *live, "状态：", false},
	}
	if *article != "" {
		if !digits.MatchString(*article) {
			panic("numeric article ID required")
		}
		cases = []contentCase{
			{"legacy-article", "https://www.bilibili.com/read/cv" + *article, "FTB Evolution", false},
			{"missing-article", "https://www.bilibili.com/read/cv0", "ERROR:", true},
			{"missing-opus", "https://www.bilibili.com/opus/0", "ERROR:", true},
			{"missing-live", "https://live.bilibili.com/0", "ERROR:", true},
			{"live-after-errors", "https://live.bilibili.com/" + *live, "状态：", false},
		}
	}
	if *mobile {
		if !digits.MatchString(*article) {
			panic("mobile checks require numeric article ID")
		}
		cases = []contentCase{
			{"mobile-article", "https://www.bilibili.com/read/mobile/" + *article + "?from=share", "FTB Evolution", false},
			{"mobile-dynamic", "https://m.bilibili.com/dynamic/" + *opus + "?from=share", "作者：", false},
			{"escaped-opus", strings.ReplaceAll("https://www.bilibili.com/opus/"+*opus, "/", `\/`), "作者：", false},
		}
	}
	for i, tc := range cases {
		if i > 0 {
			time.Sleep(12 * time.Second)
		}
		c, stop := context.WithTimeout(ctx, 25*time.Second)
		reply, _, e := h.SendWithContext(c, qa.Owner, tc.url)
		stop()
		must(e)
		check(tc.name+"-reply", strings.Contains(reply, tc.marker) && (tc.failure || !strings.Contains(reply, "ERROR:")) && !strings.Contains(reply, "no cookie config"))
		if !tc.failure {
			check(tc.name+"-image", h.ImageReceived)
		} else {
			check(tc.name+"-no-fake-image", !h.ImageReceived)
		}
		must(os.WriteFile(filepath.Join(out, tc.name+"-message.json"), h.LastSentMessage(), 0600))
	}
	_, e = os.Stat(filepath.Join(out, "data/Bilibili/config.json"))
	check("no-cookie-written", os.IsNotExist(e))
	bot.Stop()
	logs, e := os.ReadFile(filepath.Join(out, "public-stderr.log"))
	must(e)
	check("no-handler-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
	complete = true
	return passed
}
