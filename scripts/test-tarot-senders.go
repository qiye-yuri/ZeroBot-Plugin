//go:build ignore

// Isolated routing and persistence checks; no real QQ or upstream requests.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	_ "image/jpeg"
	"image/png"

	"flag"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"

	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	allFormations := flag.Bool("all-formations", false, "validate all formations using real cached images")
	binary := flag.String("binary-name", "zerobot.exe", "binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binary) != *binary {
		panic("invalid binary")
	}
	root, err := os.Getwd()
	must(err)
	must(os.MkdirAll(filepath.Join(root, ".local/tarot-senders"), 0700))
	out, err := os.MkdirTemp(filepath.Join(root, ".local/tarot-senders"), "run-")
	must(err)
	fmt.Println("ARTIFACTS=" + out)
	h, err := qa.New(root, filepath.Join(root, ".local/bin", *binary), out)
	must(err)
	defer h.Close()
	must(os.MkdirAll(filepath.Join(out, "data/Tarot"), 0700))
	for _, name := range []string{"tarots.json", "formation.json"} {
		raw, e := os.ReadFile(filepath.Join(root, "data/Tarot", name))
		must(e)
		must(os.WriteFile(filepath.Join(out, "data/Tarot", name), raw, 0600))
	}

	var pixels bytes.Buffer
	must(png.Encode(&pixels, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	var cards map[string]struct {
		Name string `json:"name"`
		Info struct {
			ImgURL string `json:"imgUrl"`
		} `json:"info"`
	}
	raw, e := os.ReadFile(filepath.Join(out, "data/Tarot/tarots.json"))
	must(e)
	must(json.Unmarshal(raw, &cards))
	for _, card := range cards {
		for _, prefix := range []string{"", "Reverse/"} {
			filename := filepath.Join(out, "data/Tarot", prefix+card.Info.ImgURL)
			must(os.MkdirAll(filepath.Dir(filename), 0700))
			data := pixels.Bytes()
			if *allFormations {
				var err error
				data, err = os.ReadFile(filepath.Join(root, "data/Tarot", prefix+card.Info.ImgURL))
				must(err)
				_, _, err = image.DecodeConfig(bytes.NewReader(data))
				must(err)
			}
			must(os.WriteFile(filename, data, 0600))
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
		c, stop := context.WithTimeout(ctx, 12*time.Second)
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
		fmt.Printf("reply %s: %d chars\n", cmd, len([]rune(r)))
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
	for _, name := range []string{"mcfish", "qqwife", "tarot"} {
		switchMain(name, "enabled")
	}

	if *allFormations {
		var formations map[string]struct {
			Count     int        `json:"cards_num"`
			Represent [][]string `json:"represent"`
		}
		raw, err := os.ReadFile(filepath.Join(out, "data/Tarot/formation.json"))
		must(err)
		must(json.Unmarshal(raw, &formations))
		names := []string{}
		for name := range formations {
			names = append(names, name)
		}
		sort.Strings(names)
		for index, name := range names {
			f := formations[name]
			for mode, kind := range []string{"塔罗", "小阿卡纳", "混合"} {
				reply := send(qa.Owner+int64(index*3+mode+1), 0, kind+"牌阵"+name, "member")
				var nodes []struct {
					Type string `json:"type"`
					Data struct {
						UIN     string `json:"uin"`
						Content string `json:"content"`
					} `json:"data"`
				}
				must(json.Unmarshal(h.LastSentMessage(), &nodes))
				identity, labels, pool := len(nodes) > 0, len(nodes) == f.Count+1, true
				images := 0
				found := map[string]bool{}
				for i, node := range nodes {
					identity = identity && node.Type == "node" && node.Data.UIN == "87654321"
					text := node.Data.Content
					images += strings.Count(text, "[CQ:image,")
					if i < f.Count {
						labels = labels && strings.Contains(text, f.Represent[0][i])
						cardID := ""
						for id, card := range cards {
							if strings.Contains(text, "』"+card.Name+"\n") {
								cardID = id
								break
							}
						}
						id, err := strconv.Atoi(cardID)
						pool = pool && err == nil && !found[cardID] && (kind != "塔罗" || id < 22) && (kind != "小阿卡纳" || id >= 22)
						found[cardID] = true
					}
				}
				prefix := kind + "-" + name
				check(prefix+"-positions", labels && strings.Contains(reply, name))
				check(prefix+"-all-images", images == f.Count)
				check(prefix+"-bot-identity", identity)
				check(prefix+"-unique-correct-pool", pool && len(found) == f.Count)
			}
		}
		bot.Stop()
		logs, err := os.ReadFile(filepath.Join(out, "bili-stderr.log"))
		must(err)
		check("no-handler-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
		completed = true
		return passed
	}
	check("draw-three", strings.Count(owner(0, "抽3张塔罗牌"), "其释义为") == 3)
	checkIdentity := func(name string) {
		var nodes []struct {
			Type string `json:"type"`
			Data struct {
				UIN  string `json:"uin"`
				Name string `json:"name"`
			} `json:"data"`
		}
		must(json.Unmarshal(h.LastSentMessage(), &nodes))
		ok := len(nodes) > 0
		for _, node := range nodes {
			ok = ok && node.Type == "node" && node.Data.UIN == "87654321" && node.Data.Name != ""
		}
		check(name, ok)
	}
	checkIdentity("draw-bot-identity")
	check("draw-cached-images", (h.ImageReceived || strings.Contains(string(h.LastSentMessage()), "[CQ:image")))
	r := owner(0, "解塔罗牌愚者")
	check("explain-fool", strings.Contains(r, "愚者的含义") && strings.Contains(r, "正位") && strings.Contains(r, "逆位"))
	check("explain-cached-image", (h.ImageReceived || strings.Contains(string(h.LastSentMessage()), "[CQ:image")))
	r = owner(0, "塔罗牌阵圣三角")
	check("formation-three-positions", strings.Contains(r, "现状") && strings.Contains(r, "愿望") && strings.Contains(r, "行动"))
	checkIdentity("formation-bot-identity")
	check("formation-cached-images", (h.ImageReceived || strings.Contains(string(h.LastSentMessage()), "[CQ:image")))
	check("next-command-unblocked", strings.Contains(owner(0, "解塔罗牌愚者"), "愚者的含义"))
	r = owner(0, "/服务列表")
	check("service-list", strings.Contains(r, "服务列表"))
	checkIdentity("service-list-bot-identity")
	check("group-formation", strings.Contains(owner(101, "塔罗牌阵圣三角"), "圣三角"))
	checkIdentity("group-formation-bot-identity")
	// Remove only synthetic card fixtures. The live data directory is never touched.
	for _, card := range cards {
		for _, prefix := range []string{"", "Reverse/"} {
			must(os.Remove(filepath.Join(out, "data/Tarot", prefix+card.Info.ImgURL)))
		}
	}
	start := time.Now()
	r = owner(0, "塔罗牌阵圣三角")
	check("uncached-formation-bounded", strings.Contains(r, "圣三角") && time.Since(start) < 12*time.Second)
	check("uncached-next-command", strings.Contains(owner(0, "解塔罗牌愚者"), "愚者的含义"))
	completed = true
	return passed
}
