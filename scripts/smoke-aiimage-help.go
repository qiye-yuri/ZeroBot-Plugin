//go:build ignore

// Uses synthetic identities, an isolated data directory and a local image API.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

var candidateBinary = flag.String("binary", ".local/bin/zerobot-aiimage-help.exe", "isolated candidate executable")

func main() {
	flag.Parse()
	absolute, err := filepath.Abs(*candidateBinary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	*candidateBinary = absolute
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	out := filepath.Join(root, ".local/aiimage-help-acceptance")
	if err = os.MkdirAll(out, 0700); err != nil {
		return err
	}
	parent := out
	out, err = os.MkdirTemp(parent, "run-")
	if err != nil {
		return err
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var v map[string]any
		if json.NewDecoder(r.Body).Decode(&v) != nil || (v["model"] != "gpt-image-2.5-flare" && v["model"] != "gpt-image-2.5-sunburst") || v["size"] != "3840x2160" || v["quality"] != "max" || v["background"] != "transparent" || v["output_format"] != "webp" || r.Header.Get("Authorization") != "Bearer synthetic-image-key" {
			http.Error(w, "bad request", 400)
			return
		}
		if v["prompt"] == "failure" {
			if v["n"] != float64(1) || v["stream"] != true || v["partial_images"] != float64(1) {
				http.Error(w, "bad streaming request", 400)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"aW1hZ2U=\"}\n\ndata: {\"type\":\"error\"}\n\n")
			return
		}
		if v["n"] != float64(2) || v["output_compression"] != float64(80) {
			http.Error(w, "bad count/compression", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"b64_json":"aW1hZ2U="},{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aOioAAAAASUVORK5CYII="}]}`)
	}))
	defer server.Close()
	h, err := qa.New(root, *candidateBinary, out)
	if err != nil {
		return err
	}
	defer h.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	bot, err := h.Start(ctx, "aiimage")
	if err != nil {
		return err
	}
	defer bot.Stop()
	c := qa.NewClient(h.Address)
	if _, err = c.Auth(h.AdminDir); err != nil {
		return err
	}
	v, err := c.Checked("GET", "/api/plugins", nil)
	if err != nil {
		return err
	}
	for _, raw := range v["plugins"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "aiimage" {
			_, err = c.Checked("POST", "/api/plugin-switch", map[string]any{"name": "aiimage", "groupId": "0", "mode": "enabled", "revision": p["state"].(map[string]any)["revision"]})
			if err != nil {
				return err
			}
		}
	}
	send := func(cmd, want string) error {
		reply, _, e := h.SendWithContext(ctx, qa.Owner, cmd)
		if e != nil {
			return e
		}
		if !strings.Contains(reply, want) {
			return fmt.Errorf("unexpected reply for %s: %s", cmd, reply)
		}
		return nil
	}
	memberLookups := h.ActionCount("get_group_member_info")
	for _, pair := range [][2]string{{"AI画图帮助", "基本格式"}, {"AI画图 --help", "不占画图预算"}, {"AI画图帮助 参数", "--compression=80"}, {"AI画图帮助 尺寸", "4k-portrait"}, {"AI画图帮助 质量", "xhigh"}, {"AI画图帮助 输出", "--moderation=auto"}, {"AI画图帮助 流式", "--partial-images=0"}, {"AI画图帮助 配置", "仅机器人主人私聊"}, {"AI画图帮助 预算", "重启"}, {"AI画图帮助 示例", "两张透明WebP"}, {"AI画图帮助 全部", "两张透明WebP"}, {"AI画图帮助 未知主题", "未找到该帮助主题"}} {
		if err = send(pair[0], pair[1]); err != nil {
			return err
		}
	}
	if h.ActionCount("get_group_member_info") != memberLookups {
		return fmt.Errorf("private help queried group member identity")
	}
	if err = send("AI画图帮助 全部", "基本格式"); err != nil {
		return err
	}
	var helpNodes []struct {
		Type string `json:"type"`
		Data struct {
			UIN  string `json:"uin"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err = json.Unmarshal(h.LastSentMessage(), &helpNodes); err != nil {
		return err
	}
	if len(helpNodes) != 10 {
		return fmt.Errorf("wrong help node count: %d", len(helpNodes))
	}
	for _, node := range helpNodes {
		if node.Type != "node" || node.Data.UIN != "87654321" || node.Data.Name == "" || node.Data.Name == "Synthetic acceptance user" {
			return fmt.Errorf("help not attributed to bot")
		}
	}
	if h.ActionCount("get_group_member_info") != memberLookups {
		return fmt.Errorf("merged help queried group member identity")
	}
	replyToMember, _, err := h.SendWithContext(ctx, qa.Owner+1, "AI画图帮助 尺寸")
	if err != nil || !strings.Contains(replyToMember, "3840") {
		return fmt.Errorf("ordinary user cannot read help: %v", err)
	}
	if calls.Load() != 0 {
		return fmt.Errorf("help invoked model")
	}
	if _, err = os.Stat(filepath.Join(h.Root, "data/aiimage/aiimage.db")); !os.IsNotExist(err) {
		return fmt.Errorf("help initialized image DB: %v", err)
	}
	imageBefore, err := c.Checked("GET", "/api/ai-image", nil)
	if err != nil {
		return err
	}
	for _, model := range []string{"gpt-image-2.5-flare", "gpt-image-2.5-sunburst"} {
		imageBefore, err = c.Checked("POST", "/api/ai-image", map[string]any{"revision": imageBefore["revision"], "mode": "openai", "url": "https://api.openai.com/v1/images/generations", "model": model, "key": map[string]any{"action": "keep"}})
		if err != nil {
			return err
		}
		if imageBefore["pricingReady"] != true || imageBefore["quality"] != "medium" || imageBefore["monthlyBudgetUSD"] != "100.000000" {
			return fmt.Errorf("2.5 official defaults or pricing unavailable: %s", model)
		}
	}
	saved, err := c.Checked("POST", "/api/ai-image", map[string]any{"revision": imageBefore["revision"], "mode": "openai", "url": server.URL, "model": "gpt-image-2.5-flare", "quality": "medium", "size": "1024x1024", "monthlyBudgetUSD": "0.3", "customCostUSD": "0.1", "key": map[string]any{"action": "replace", "value": "synthetic-image-key"}})
	if err != nil {
		return err
	}
	if saved["keyConfigured"] != true || saved["model"] != "gpt-image-2.5-flare" {
		return fmt.Errorf("admin config not saved")
	}
	raw, _ := json.Marshal(saved)
	if strings.Contains(string(raw), "synthetic-image-key") {
		return fmt.Errorf("key exposed")
	}
	snapshots, _ := filepath.Glob(filepath.Join(h.AdminDir, "imagegen-backups", "*.sqlite"))
	if len(snapshots) != 3 {
		return fmt.Errorf("backup missing")
	}
	if err = send("查看AI画图配置", "gpt-image-2.5-flare"); err != nil {
		return err
	}
	for _, pair := range [][2]string{

		{"查看AI画图配置", "已设置（隐藏）"},
		{"AI画图 --size=4k --quality=max --n=2 --background=transparent --format=webp --compression=80 synthetic image", "正在生成"},
	} {
		if err = send(pair[0], pair[1]); err != nil {
			return err
		}
	}
	reply, err := h.AwaitReply(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(reply, "图片生成完成") || !strings.Contains(string(h.LastSentMessage()), "base64://") {
		return fmt.Errorf("image not sent: %s", reply)
	}
	current, err := c.Checked("GET", "/api/ai-image", nil)
	if err != nil {
		return err
	}
	_, err = c.Checked("POST", "/api/ai-image", map[string]any{"revision": current["revision"], "mode": "openai", "url": server.URL, "model": "gpt-image-2.5-sunburst", "customCostUSD": "0.1", "key": map[string]any{"action": "keep"}})
	if err != nil {
		return err
	}
	if err = send("AI画图 --size=4k --quality=max --background=transparent --format=webp --stream=true --partial-images=1 failure", "正在生成"); err != nil {
		return err
	}
	reply, err = h.AwaitReply(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(reply, "生成预览") {
		return fmt.Errorf("stream preview not sent")
	}
	reply, err = h.AwaitReply(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(reply, "流式生成失败") || strings.Contains(reply, "生成完成") {
		return fmt.Errorf("bad failure response: %s", reply)
	}
	if calls.Load() != 2 {
		return fmt.Errorf("unexpected calls %d", calls.Load())
	}
	if err = send("AI画图 one more", "预算不足"); err != nil {
		return err
	}
	if calls.Load() != 2 {
		return fmt.Errorf("budget rejection made a paid request")
	}
	budgetView, err := c.Checked("GET", "/api/ai-image", nil)
	if err != nil {
		return err
	}
	budget := budgetView["budget"].(map[string]any)
	if budget["chargedUSD"] != "0.300000" || budget["remainingUSD"] != "0.000000" || budget["stopped"] != true {
		return fmt.Errorf("budget summary mismatch: %v", budget)
	}
	if err = send("AI画图帮助 全部", "基本格式"); err != nil {
		return err
	}
	if calls.Load() != 2 {
		return fmt.Errorf("help at exhausted budget invoked model")
	}
	afterHelp, err := c.Checked("GET", "/api/ai-image", nil)
	if err != nil {
		return err
	}
	if afterHelp["budget"].(map[string]any)["chargedUSD"] != "0.300000" {
		return fmt.Errorf("help consumed budget")
	}
	// Restart the isolated candidate; its persisted quota must still reject.
	bot.Stop()
	bot, err = h.Start(ctx, "aiimage-restarted")
	if err != nil {
		return err
	}
	defer bot.Stop()
	if err = send("AI画图 after restart", "预算不足"); err != nil {
		return err
	}
	if calls.Load() != 2 {
		return fmt.Errorf("restart reset budget")
	}
	if _, err = c.Auth(h.AdminDir); err != nil {
		return err
	}
	if err = send("清除AI画图密钥", "已清除"); err != nil {
		return err
	}
	if err = send("查看AI画图配置", "未设置"); err != nil {
		return err
	}
	afterClear, err := c.Checked("GET", "/api/ai-image", nil)
	if err != nil {
		return err
	}
	if afterClear["keyConfigured"] != false || afterClear["revision"] == saved["revision"] {
		return fmt.Errorf("chat changes not visible to admin")
	}
	result := map[string]any{"passed": true, "isolated": true, "realQQMessagesSent": false, "realModelCalls": 0, "syntheticImageCalls": calls.Load(), "imagePayloadVerified": true, "errorReplyVerified": true, "budgetStopVerified": true, "image25OfficialPricingReady": true, "defaultMedium100Preserved": true, "restartPreservesBudget": true, "qualityAndDimensionsVerified": true, "options4KMultipleVerified": true, "streamPreviewVerified": true, "helpNoModelOrBudget": true, "helpBeforeConfiguration": true, "helpTopicsAndForwardVerified": true, "helpBotSenderVerified": true, "noPrivateGroupLookup": true}
	b, _ := json.MarshalIndent(result, "", "  ")
	if err = os.WriteFile(filepath.Join(parent, "integration-result.json"), b, 0600); err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
