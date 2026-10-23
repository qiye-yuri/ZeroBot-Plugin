//go:build ignore

// Synthetic QQ and dictionaries; English definitions may use the bounded translation API.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	"image"
	_ "image/jpeg"
	_ "image/png"
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
	bin := flag.String("binary-name", "zerobot-word-games.exe", "binary name")
	boundaries := flag.Bool("answer-boundaries", false, "test invalid answers, exhausted guesses and six/seven letter modes")
	rooms := flag.Bool("room-isolation", false, "test simultaneous games in separate groups")
	lifecycle := flag.Bool("lifecycle", false, "test idle timeout and room release")
	realResources := flag.Bool("real-resources", false, "use staged public dictionaries for startup checks")
	flag.Parse()
	if filepath.Base(*bin) != *bin {
		panic("invalid binary")
	}
	root, e := os.Getwd()
	must(e)
	parent := filepath.Join(root, ".local/word-games")
	must(os.MkdirAll(parent, 0700))
	out, e := os.MkdirTemp(parent, "run-")
	must(e)
	fmt.Println("ARTIFACTS=" + out)
	h, e := qa.New(root, filepath.Join(root, ".local/bin", *bin), out)
	must(e)
	defer h.Close()
	write := func(name, s string) {
		p := filepath.Join(out, name)
		must(os.MkdirAll(filepath.Dir(p), 0700))
		must(os.WriteFile(p, []byte(s), 0600))
	}
	for size, word := range map[int]string{5: "apple", 6: "banana", 7: "example"} {
		for _, prefix := range []string{"cet-4_", "dict_"} {
			write(fmt.Sprintf("data/Wordle/%s%d.txt", prefix, size), word+"\r\n\n")
		}
	}
	if *boundaries {
		write("data/Wordle/dict_5.txt", "apple\r\ngrape\r\n")
	}
	write("data/Handou/idiom.json", `{"一心一意":{"word":"一心一意","chars":["一","心","一","意"],"pinyin":["yī","xīn","yī","yì"],"explanation":"专心一致"}}`)
	write("data/Handou/userHabits.json", `{"habits":{},"bigrams":{},"idioms":{},"total_words":0,"total_idioms":0}`)
	if *realResources {
		for _, name := range []string{"Handou/idiom.json", "Wordle/cet-4_5.txt", "Wordle/dict_5.txt", "Wordle/cet-4_6.txt", "Wordle/dict_6.txt", "Wordle/cet-4_7.txt", "Wordle/dict_7.txt"} {
			data, err := os.ReadFile(filepath.Join(root, ".local/word-games/resources", name))
			must(err)
			write("data/"+name, string(data))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	bot, e := h.Start(ctx, "games")
	must(e)
	defer bot.Stop()
	api := qa.NewClient(h.Address)
	_, e = api.Auth(h.AdminDir)
	must(e)
	passed, completed := true, false
	checks := []map[string]any{}
	defer func() {
		must(qa.WriteJSON(filepath.Join(out, "result.json"), map[string]any{"passed": passed && completed, "checks": checks, "realQQMessagesSent": false, "syntheticDictionaries": !*realResources, "translationMayUseNetwork": true}))
	}()
	check := func(n string, ok bool) {
		checks = append(checks, map[string]any{"name": n, "passed": ok})
		passed = passed && ok
		fmt.Printf("%s=%v\n", n, ok)
	}
	send := func(uid, gid int64, text, role string, wait time.Duration) string {
		c, stop := context.WithTimeout(ctx, wait)
		defer stop()
		r, e := h.SendGroupAs(c, uid, gid, text, role)
		if e != context.DeadlineExceeded {
			must(e)
		}
		return r
	}
	owner := func(gid int64, text string) string {
		// This suite deliberately issues several blocked starts: these also count
		// toward the shared per-user rate limit (five requests per ten seconds).
		if *rooms {
			time.Sleep(2200 * time.Millisecond)
		}
		return send(qa.Owner, gid, text, "owner", 25*time.Second)
	}
	if *rooms {
		check("room-a-english-start", strings.Contains(owner(301, "个人猜词 英文"), "长度为5"))
		check("room-b-chinese-start", strings.Contains(owner(302, "个人猜词 中文"), "次机会"))
		check("room-a-conflicting-mode-blocked", strings.Contains(owner(301, "团队猜词 中文"), "正在进行"))
		check("room-b-still-busy", strings.Contains(owner(302, "个人猜词 英文"), "正在进行"))
		check("room-a-rejects-unknown", strings.Contains(owner(301, "zzzzz"), "存在这样的单词"))
		check("room-a-stop", strings.Contains(owner(301, "结束猜词"), "猜单词已结束"))
		check("room-b-not-stopped", strings.Contains(owner(302, "个人猜词 英文"), "正在进行"))
		check("room-a-reopens-chinese", strings.Contains(owner(301, "个人猜词 中文"), "次机会"))
		check("room-b-win", strings.Contains(owner(302, "一心一意"), "太棒了"))
		check("room-a-survives-other-win", strings.Contains(owner(301, "团队猜词 英文"), "正在进行"))
		check("room-a-final-stop", strings.Contains(owner(301, "结束猜词"), "猜成语已结束"))
		quiet, done := context.WithTimeout(ctx, 12*time.Second)
		reply, err := h.AwaitReply(quiet)
		done()
		check("no-extra-after-both-finished", reply == "" && err == context.DeadlineExceeded)
		bot.Stop()
		logs, err := os.ReadFile(filepath.Join(out, "games-stderr.log"))
		must(err)
		check("no-handler-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
		completed = true
		return passed
	}
	if *boundaries {
		check("invalid-start", strings.Contains(owner(201, "个人猜词 英文"), "长度为5"))
		for i := 0; i < 7; i++ {
			check(fmt.Sprintf("unknown-does-not-end-%d", i+1), strings.Contains(owner(201, "zzzzz"), "存在这样的单词"))
		}
		check("unknown-keeps-winning-opportunity", strings.Contains(owner(201, "APPLE"), "太棒了"))
		check("exhaustion-start", strings.Contains(owner(202, "个人猜词 英文"), "长度为5"))
		for i := 0; i < 6; i++ {
			reply := owner(202, "grape")
			if i < 5 {
				check(fmt.Sprintf("valid-wrong-image-%d", i+1), validImageMessage(h.LastSentMessage()) && !strings.Contains(reply, "游戏结束"))
			} else {
				check("sixth-wrong-ends-with-answer", strings.Contains(reply, "游戏结束") && strings.Contains(reply, "apple"))
			}
		}
		check("exhausted-room-reopens", strings.Contains(owner(202, "个人猜词 中文"), "次机会"))
		check("reopened-room-stops", strings.Contains(owner(202, "结束猜词"), "已结束"))
		for _, item := range []struct {
			group                            int64
			command, unknown, answer, length string
		}{{203, "个人六阶猜词 英文", "zzzzzz", "BANANA", "长度为6"}, {204, "个人七阶猜词 英文", "zzzzzzz", "EXAMPLE", "长度为7"}} {
			check(item.length+"-start", strings.Contains(owner(item.group, item.command), item.length))
			check(item.length+"-unknown", strings.Contains(owner(item.group, item.unknown), "存在这样的单词"))
			check(item.length+"-win", strings.Contains(owner(item.group, item.answer), "太棒了"))
		}
		bot.Stop()
		logs, err := os.ReadFile(filepath.Join(out, "games-stderr.log"))
		must(err)
		check("no-handler-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
		completed = true
		return passed
	}
	if *lifecycle {
		check("idle-chinese-start", strings.Contains(owner(101, "个人猜词 中文"), "次机会"))
		chineseAt := time.Now()
		check("idle-english-start", strings.Contains(owner(102, "个人猜词 英文"), "长度为5"))
		englishAt := time.Now()
		seen := map[string]int{}
		deadline, stop := context.WithTimeout(ctx, 135*time.Second)
		for len(seen) < 4 {
			reply, err := h.AwaitReply(deadline)
			if err == context.DeadlineExceeded {
				break
			}
			must(err)
			for _, mode := range []string{"猜成语", "猜单词"} {
				start := chineseAt
				if mode == "猜单词" {
					start = englishAt
				}
				kind := ""
				minimum := 0.0
				if strings.Contains(reply, mode+"，你还有15s") {
					kind = "reminder"
					minimum = 100
				}
				if strings.Contains(reply, mode+"超时") {
					kind = "timeout"
					minimum = 115
				}
				if kind != "" {
					seen[mode+kind]++
					check(mode+kind+"-not-early", time.Since(start).Seconds() >= minimum)
				}
			}
		}
		stop()
		for _, name := range []string{"猜成语reminder", "猜成语timeout", "猜单词reminder", "猜单词timeout"} {
			check(name+"-once", seen[name] == 1)
		}
		check("chinese-room-released", strings.Contains(owner(101, "个人猜词 中文"), "次机会"))
		check("chinese-explicit-stop", strings.Contains(owner(101, "结束猜词"), "已结束"))
		check("english-room-released", strings.Contains(owner(102, "个人猜词 英文"), "长度为5"))
		check("english-explicit-stop", strings.Contains(owner(102, "结束猜词"), "已结束"))
		quiet, done := context.WithTimeout(ctx, 10*time.Second)
		reply, err := h.AwaitReply(quiet)
		done()
		check("no-extra-replies-after-stop", reply == "" && err == context.DeadlineExceeded)
		bot.Stop()
		logs, err := os.ReadFile(filepath.Join(out, "games-stderr.log"))
		must(err)
		check("no-handler-panic", !strings.Contains(string(logs), "execute handler err") && !strings.Contains(string(logs), "panic:"))
		completed = true
		return passed
	}
	if *realResources {
		check("public-chinese-start", strings.Contains(owner(101, "个人猜词 中文"), "次机会"))
		check("public-chinese-stop", strings.Contains(owner(101, "结束猜词"), "已结束"))
		check("public-english-start", strings.Contains(owner(102, "团队猜词 英文"), "长度为5"))
		check("public-english-stop", strings.Contains(owner(102, "结束猜词"), "已结束"))
		completed = true
		return passed
	}
	check("chinese-unified-start", strings.Contains(owner(101, "个人猜词 中文"), "次机会"))
	check("same-group-cross-mode-busy", strings.Contains(owner(101, "团队猜单词"), "正在进行"))
	check("personal-other-user-ignored", send(23456789, 101, "一心一意", "member", time.Second) == "")
	check("self-message-ignored", send(87654321, 101, "一心一意", "member", time.Second) == "")
	check("chinese-win", strings.Contains(owner(101, "一心一意"), "太棒了"))
	check("english-unified-start", strings.Contains(owner(102, "团队猜词 英文"), "长度为5"))
	check("english-team-member-win", strings.Contains(send(23456789, 102, "APPLE", "member", 3*time.Second), "太棒了"))
	check("chinese-old-alias", strings.Contains(owner(103, "团队猜成语"), "次机会"))
	check("unrelated-member-cannot-stop", send(23456789, 103, "结束猜词", "member", time.Second) == "")
	check("team-admin-stop", strings.Contains(send(34567890, 103, "结束猜词", "admin", 3*time.Second), "已结束"))
	check("english-six-start", strings.Contains(owner(104, "个人六阶猜单词"), "长度为6"))
	check("english-other-user-ignored", send(23456789, 104, "banana", "member", time.Second) == "")
	check("english-starter-stop", strings.Contains(owner(104, "结束猜词"), "已结束"))
	check("english-seven-start", strings.Contains(send(45678901, 105, "团队七阶猜单词", "member", 25*time.Second), "长度为7"))
	check("english-seven-win", strings.Contains(owner(105, "example"), "太棒了"))
	catalog, e := api.Checked("GET", "/api/plugins", nil)
	must(e)
	for _, raw := range catalog["plugins"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "wordle" {
			_, e = api.Checked("POST", "/api/plugin-switch", map[string]any{"name": "wordle", "groupId": "0", "mode": "disabled", "revision": p["state"].(map[string]any)["revision"]})
			must(e)
		}
	}
	for _, command := range []string{"个人猜成语", "团队猜单词", "个人猜词 中文", "团队猜词 英文"} {
		check("unified-disabled-"+command, send(qa.Owner, 106, command, "owner", time.Second) == "")
	}
	habits, e := os.ReadFile(filepath.Join(out, "data/Handou/userHabits.json"))
	must(e)
	check("legacy-habits-updated", strings.Contains(string(habits), "一心一意"))
	completed = true
	return passed
}

func validImageMessage(raw json.RawMessage) bool {
	var segments []struct {
		Type string `json:"type"`
		Data struct {
			File string `json:"file"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &segments) != nil {
		return false
	}
	for _, s := range segments {
		if s.Type != "image" || !strings.HasPrefix(s.Data.File, "base64://") {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s.Data.File, "base64://"))
		if err != nil {
			return false
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		return err == nil && cfg.Width > 0 && cfg.Height > 0
	}
	return false
}
