//go:build ignore

// Run on Windows with go run scripts/test-plugin-basics.go.
// Uses the deployed binary with synthetic OneBot and a separate data directory.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
)

type check struct {
	Name   string `json:"name"`
	Plugin string `json:"plugin"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

func main() {
	if !run() {
		os.Exit(1)
	}
}

func run() bool {
	binaryName := flag.String("binary-name", "zerobot.exe", "tested binary under .local/bin")
	flag.Parse()
	if filepath.Base(*binaryName) != *binaryName || filepath.Ext(*binaryName) != ".exe" {
		panic("invalid binary name")
	}
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	parent := filepath.Join(root, ".local", "plugin-basics")
	if err = os.MkdirAll(parent, 0700); err != nil {
		panic(err)
	}
	out, err := os.MkdirTemp(parent, "run-")
	if err != nil {
		panic(err)
	}
	fmt.Println("ARTIFACTS=" + out)
	checks := []check{}
	report := map[string]any{"passed": false, "isolated": true, "realQQMessagesSent": false, "dailyConfigurationWrites": false, "checks": checks, "startedAt": time.Now().Format(time.RFC3339)}
	defer func() {
		report["checks"] = checks
		report["completedAt"] = time.Now().Format(time.RFC3339)
		qa.WriteJSON(filepath.Join(out, "result.json"), report)
	}()
	must := func(err error) {
		if err != nil {
			report["error"] = err.Error()
			panic(err)
		}
	}
	binary := filepath.Join(root, ".local/bin", *binaryName)
	raw, err := os.ReadFile(binary)
	must(err)
	sum := sha256.Sum256(raw)
	report["binarySHA256"] = hex.EncodeToString(sum[:])
	h, err := qa.New(root, binary, out)
	must(err)
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	bot, err := h.Start(ctx, "plugins")
	must(err)
	defer bot.Stop()
	client := qa.NewClient(h.Address)
	_, err = client.Auth(h.AdminDir)
	must(err)
	catalog, err := client.Checked("GET", "/api/plugins", nil)
	must(err)
	must(qa.WriteJSON(filepath.Join(out, "catalog.json"), catalog))
	names := map[string]bool{}
	for _, p := range catalog["plugins"].([]any) {
		row := p.(map[string]any)
		names[row["name"].(string)] = true
	}
	raw, err = os.ReadFile(filepath.Join(root, "main.go"))
	must(err)
	re := regexp.MustCompile(`(?m)^\s*_ "github.com/FloatTech/ZeroBot-Plugin/plugin/([^"/]+)"`)
	for _, m := range re.FindAllSubmatch(raw, -1) {
		name := string(m[1])
		checks = append(checks, check{"registered", name, names[name], "runtime registry only; not business acceptance"})
	}
	removed := []string{"bookreview", "ygocdb", "ygotrade", "gif", "midicreate", "qzone", "tiangou", "alipayvoice", "nihongo", "pig", "moyucalendar", "funny", "xhstext", "base64gua", "baseamasiro", "shadiao", "atri", "ahsai", "lolimi", "yujn", "magicprompt", "curse", "thesaurus", "llm", "aichatcfg", "aiwife", "setutime", "coser", "nativesetu", "lolicon", "nwife", "danbooru", "nsfw", "realcugan", "tracemoe", "bilibili", "bilibilipush", "rsshub", "steam", "drawlots", "shindan", "reborn", "poker", "omikuji", "guessmusic", "genshin", "chess", "niuniu", "robbery", "handou", "wallet", "wordcount", "base16384", "chouxianghua", "chrev", "crypter", "emozi", "font", "translation", "cpstory", "diana", "hitokoto", "kfccrazythursday", "wantquotes", "dailynews", "dish", "hyaku", "jptingroom", "movies", "novel", "hs", "warframeapi", "ymgal", "driftbottle", "emojimix", "jandan", "moyu", "music", "choose", "github", "nbnhhsh", "runcode", "chatcount", "sleepmanage", "autowithdraw", "baiduaudit", "inject", "fortune", "animetrace", "saucenao"}
	for _, name := range removed {
		checks = append(checks, check{"removed-absent", name, !names[name], ""})
	}
	send := func(cmd string) string { reply, _, err := h.Send(qa.Owner, cmd); must(err); return reply }
	add := func(name, plugin string, passed bool, detail string) {
		checks = append(checks, check{name, plugin, passed, detail})
		fmt.Printf("%s/%s passed=%v\n", plugin, name, passed)
	}
	pages := (len(names) + 19) / 20
	var allPages strings.Builder
	for page := 1; page <= pages; page++ {
		reply := send(fmt.Sprintf("/服务列表 %d", page))
		add(fmt.Sprintf("page-%d", page), "service-list",
			strings.Contains(reply, fmt.Sprintf("第 %d/%d 页", page, pages)), "")
		allPages.WriteString(reply)
	}
	for _, name := range removed {
		add("removed-from-all-pages", name, !strings.Contains(allPages.String(), "插件："+name+"\n"), "")
	}
	for name := range names {
		add("retained-in-pages", name, strings.Contains(allPages.String(), "插件："+name+"\n"), "")
	}
	// Integrated AI entry points share one gate. Group interactions keep their
	// independent switches; no live QQ account or model is used here.
	switchPlugin := func(name, mode string) {
		v, e := client.Checked("GET", "/api/plugins", nil)
		must(e)
		for _, raw := range v["plugins"].([]any) {
			p := raw.(map[string]any)
			if p["name"] == name {
				_, e = client.Checked("POST", "/api/plugin-switch", map[string]any{
					"name": name, "groupId": "0", "mode": mode,
					"revision": p["state"].(map[string]any)["revision"],
				})
				must(e)
				return
			}
		}
		panic("missing plugin " + name)
	}
	switchPlugin("aichat", "disabled")

	for _, command := range []string{"/gpt", "查看AI聊天配置", "空调开", "空调关", "群温度", "设置温度25"} {
		add("disabled-or-removed-"+command, "aichat/chat", send(command) == "", "")
	}
	poke := func() string {
		c, cancel := context.WithTimeout(ctx, 1800*time.Millisecond)
		defer cancel()
		r, e := h.SendPoke(c, qa.Owner, 101)
		if e != context.DeadlineExceeded {
			must(e)
		}
		return r
	}
	repeat := func(text string) string {
		reply := ""
		for i := 0; i < 5; i++ {
			c, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
			r, e := h.SendGroupWithContext(c, qa.Owner, 101, text, "")
			cancel()
			if e != context.DeadlineExceeded {
				must(e)
			}
			reply += r
		}
		return reply
	}
	switchPlugin("chat", "enabled")
	switchPlugin("breakrepeat", "disabled")
	add("poke-with-ai-repeat-off", "chat", strings.Contains(poke(), "戳"), "")
	add("repeat-disabled", "breakrepeat", repeat("复读验收甲ABCDE") == "", "")
	switchPlugin("chat", "disabled")
	switchPlugin("breakrepeat", "enabled")
	add("poke-disabled", "chat", poke() == "", "")
	add("repeat-with-ai-poke-off", "breakrepeat", repeat("复读验收乙FGHIJ") != "", "")
	switchPlugin("chat", "enabled")
	switchPlugin("breakrepeat", "disabled")
	switchPlugin("aichat", "enabled")
	add("unified-gpt-command", "aichat", strings.Contains(send("/gpt"), "填写问题"), "")
	add("unified-config-command", "aichat", send("查看AI聊天配置") != "", "")
	// Disable AI fallback to verify that removed commands have no remaining handlers.
	switchPlugin("aichat", "disabled")
	for _, c := range []struct{ name, cmd string }{
		{"saucenao", "搜来源"}, {"saucenao", "搜番"}, {"saucenao", "开启搜图显示图片"}, {"fortune", "运势"}, {"fortune", "抽签"}, {"fortune", "设置底图车万"},
		{"base16384", "加密验收"}, {"base16384", "解密验收"},
		{"chouxianghua", "抽象翻译你好"}, {"chrev", "翻转 abc"},
		{"crypter", "齁语解密 呼呼"}, {"crypter", "fumo加密验收"}, {"crypter", "qq加密验收"},
		{"emozi", "抽象转写验收"}, {"emozi", "抽象还原验收"},
		{"font", "渲染文字验收"}, {"translation", ">TL good"},
		{"cpstory", "磕cp测试甲 测试乙"}, {"diana", "小作文"}, {"diana", "发大病"},
		{"hitokoto", "一言"}, {"hitokoto", "系列一言"},
		{"kfccrazythursday", "疯狂星期四"}, {"wantquotes", "据意查句 大海"}, {"wantquotes", "登录据意查句"},
		{"dailynews", "今日早报"}, {"dish", "随机菜谱"}, {"hyaku", "百人一首之1"},
		{"jptingroom", "随机日语听力"}, {"movies", "今日电影"}, {"novel", "小说测试"},
		{"hs", "搜卡测试"}, {"warframeapi", "wf警报"}, {"warframeapi", ".wm 测试"},
		{"ymgal", "随机galCG"}, {"ymgal", "更新gal"},
		{"emojimix", "😀😎"}, {"jandan", "来份吊图"}, {"moyu", "摸鱼提醒"}, {"music", "点歌 稻香"},
		{"choose", "选择苹果还是香蕉"}, {"github", ">github -t repo:golang/go"}, {"nbnhhsh", "?? yyds"}, {"runcode", ">runcode python print(12345)"},
	} {
		add("removed-command-silent", c.name, send(c.cmd) == "", "AI disabled; synthetic QQ only")
	}

	for _, cmd := range []string{"pick", "throw 这是一条合成测试漂流瓶内容"} {
		c, cancel := context.WithTimeout(ctx, 1800*time.Millisecond)
		reply, e := h.SendGroupWithContext(c, qa.Owner, 101, cmd, "")
		cancel()
		if e != context.DeadlineExceeded {
			must(e)
		}
		add("removed-group-command-silent", "driftbottle", reply == "", "synthetic group, AI disabled")
	}

	for _, item := range []struct{ name, cmd string }{{"chatcount", "查询水群"}, {"chatcount", "查看水群排名"}, {"sleepmanage", "早安"}, {"sleepmanage", "晚安"}} {
		c, cancel := context.WithTimeout(ctx, 1800*time.Millisecond)
		reply, e := h.SendGroupWithContext(c, qa.Owner, 101, item.cmd, "")
		cancel()
		if e != context.DeadlineExceeded {
			must(e)
		}
		add("removed-group-command-silent", item.name, reply == "", "synthetic group; AI off; registry absence also checked")
	}
	passed := true
	failed := 0
	for _, c := range checks {
		if !c.Passed {
			passed = false
			failed++
		}
	}
	report["passed"] = passed
	report["failedChecks"] = failed
	report["registeredCount"] = len(names)
	fmt.Printf("checks=%d failed=%d\n", len(checks), failed)
	return passed
}
