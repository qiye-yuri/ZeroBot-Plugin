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

var candidateBinary = flag.String("binary", ".local/bin/zerobot-aiimage-admin.exe", "isolated candidate executable")

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
	out := filepath.Join(root, ".local/aiimage-admin-acceptance")
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
		if json.NewDecoder(r.Body).Decode(&v) != nil || v["n"] != float64(1) || v["size"] != "1024x1024" || r.Header.Get("Authorization") != "Bearer synthetic-image-key" {
			http.Error(w, "bad request", 400)
			return
		}
		if v["prompt"] == "failure" {
			http.Error(w, "synthetic error", 429)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aOioAAAAASUVORK5CYII="}]}`)
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
	imageBefore, err := c.Checked("GET", "/api/ai-image", nil)
	if err != nil {
		return err
	}
	saved, err := c.Checked("POST", "/api/ai-image", map[string]any{"revision": imageBefore["revision"], "mode": "openai", "url": server.URL, "model": "synthetic-model", "customCostUSD": "0", "key": map[string]any{"action": "replace", "value": "synthetic-image-key"}})
	if err != nil {
		return err
	}
	if saved["keyConfigured"] != true || saved["model"] != "synthetic-model" {
		return fmt.Errorf("admin config not saved")
	}
	raw, _ := json.Marshal(saved)
	if strings.Contains(string(raw), "synthetic-image-key") {
		return fmt.Errorf("key exposed")
	}
	snapshots, _ := filepath.Glob(filepath.Join(h.AdminDir, "imagegen-backups", "*.sqlite"))
	if len(snapshots) != 1 {
		return fmt.Errorf("backup missing")
	}
	if err = send("查看AI画图配置", "synthetic-model"); err != nil {
		return err
	}
	for _, pair := range [][2]string{

		{"查看AI画图配置", "已设置（隐藏）"},
		{"AI画图 synthetic image", "正在生成"},
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
	if err = send("AI画图 failure", "正在生成"); err != nil {
		return err
	}
	reply, err = h.AwaitReply(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(reply, "HTTP 429") || strings.Contains(reply, "生成完成") {
		return fmt.Errorf("bad failure response: %s", reply)
	}
	if calls.Load() != 2 {
		return fmt.Errorf("unexpected calls %d", calls.Load())
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
	result := map[string]any{"passed": true, "isolated": true, "realQQMessagesSent": false, "realModelCalls": 0, "syntheticImageCalls": calls.Load(), "imagePayloadVerified": true, "errorReplyVerified": true}
	b, _ := json.MarshalIndent(result, "", "  ")
	if err = os.WriteFile(filepath.Join(parent, "integration-result.json"), b, 0600); err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
