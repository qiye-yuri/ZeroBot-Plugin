//go:build ignore

// Isolated routing and persistence checks; no real QQ or upstream requests.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"

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
	binary := flag.String("binary-name", "zerobot-games-check.exe", "binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(filepath.Join(root, ".local/games-check"), 0700))
	out, err := os.MkdirTemp(filepath.Join(root, ".local/games-check"), "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
	must(err)
	defer h.Close()
	for _, name := range []string{"regular-bold.ttf", "regular.ttf"} {
		raw, e := os.ReadFile(filepath.Join(root, "data/Font", name))
		must(e)
		must(os.WriteFile(filepath.Join(out, "data/Font", name), raw, 0600))
	}

	// Public fixture metadata and synthetic PNGs exercise routing without remote downloads.
	for _, rel := range []string{"data/Tarot/tarots.json", "data/Tarot/formation.json"} {
		raw, e := os.ReadFile(filepath.Join(root, rel))
		must(e)
		must(os.MkdirAll(filepath.Dir(filepath.Join(out, rel)), 0700))
		must(os.WriteFile(filepath.Join(out, rel), raw, 0600))
	}
	raw, err := os.ReadFile(filepath.Join(out, "data/Tarot/tarots.json"))
	must(err)
	cards := map[string]struct {
		Name string `json:"name"`
		Info struct {
			ImgURL string `json:"imgUrl"`
		} `json:"info"`
	}{}
	must(json.Unmarshal(raw, &cards))
	png, e := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a3ioAAAAASUVORK5CYII=")
	must(e)
	for _, card := range cards {
		for _, prefix := range []string{"", "Reverse/"} {
			p := filepath.Join(out, "data/Tarot", prefix+card.Info.ImgURL)
			must(os.MkdirAll(filepath.Dir(p), 0700))
			must(os.WriteFile(p, png, 0600))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
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
		time.Sleep(500 * time.Millisecond)
		duration := 4 * time.Second
		if cmd == "钓鱼背包" || strings.HasPrefix(cmd, "抽") || strings.HasPrefix(cmd, "解塔罗") {
			duration = 30 * time.Second
		}
		c, stop := context.WithTimeout(ctx, duration)
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
	for _, name := range []string{"mcfish", "qqwife", "tarot"} {
		switchMain(name, "enabled")
	}
	check("fish-rules", strings.Contains(owner(0, "查看钓鱼规则"), "钓鱼"))
	r := owner(0, "钓鱼背包")
	check("fish-empty-pack-image", h.ImageReceived && !strings.Contains(r, "ERROR"))
	r = owner(0, "进行钓鱼")
	fmt.Printf("purchase-prompt=%q\n", r)
	check("fish-offers-purchase", strings.Contains(r, "100") && strings.Contains(r, "是否"))
	check("other-session-cannot-cancel", owner(101, "否") == "")
	check("original-session-cancels", strings.Contains(owner(0, "否"), "已取消购买"))
	time.Sleep(2100 * time.Millisecond)
	check("marriage-empty-list", strings.Contains(owner(101, "群老婆列表"), "还没有人"))
	check("marriage-empty-members-safe", strings.Contains(owner(101, "娶群友"), "没有ta人"))
	check("marriage-private-ignored", owner(0, "娶群友") == "")
	check("member-reset-denied", send(23456789, 101, "重置所有花名册", "member") == "")
	time.Sleep(2100 * time.Millisecond)
	check("tarot-zero-rejected", strings.Contains(owner(0, "抽0张塔罗牌"), "张数必须为正"))
	check("tarot-excess-rejected", strings.Contains(owner(0, "抽21张塔罗牌"), "过多"))
	check("minor-deck-complete", len(cards) == 78 && cards["77"].Name != "")
	r = owner(0, "抽3张小阿卡纳")
	check("tarot-three-cards", strings.Count(r, "其释义为") == 3 || strings.Count(r, "『") >= 6)
	time.Sleep(2100 * time.Millisecond)
	r = owner(0, "解塔罗牌"+cards["77"].Name)
	check("tarot-last-card-interpretation", strings.Contains(r, "正位") && strings.Contains(r, "逆位") && h.ImageReceived)
	for _, item := range []struct {
		name, cmd string
		gid       int64
	}{{"mcfish", "查看钓鱼规则", 0}, {"qqwife", "群老婆列表", 101}, {"tarot", "抽0张塔罗牌", 0}} {
		switchMain(item.name, "disabled")
		check("disabled-"+item.name, owner(item.gid, item.cmd) == "")
	}
	completed = true
	return passed
}
