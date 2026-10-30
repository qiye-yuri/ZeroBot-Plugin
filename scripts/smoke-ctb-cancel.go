//go:build ignore

// Synthetic long-running downloader and isolated OneBot; no network downloads.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	ctb "github.com/FloatTech/ZeroBot-Plugin/internal/ctb"
)

var binary = flag.String("binary", ".local/bin/zerobot-ctb-cancel.exe", "candidate executable")

func main() {
	if len(os.Args) > 1 && os.Args[1] == "inspect" {
		fmt.Println("synthetic query after cancellation")
		return
	}
	var err error
	if len(os.Args) > 1 && os.Args[1] == "download" {
		err = downloadFixture()
	} else {
		flag.Parse()
		err = run()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func downloadFixture() error {
	dir := ""
	for i, arg := range os.Args {
		if arg == "--output" && i+1 < len(os.Args) {
			dir = os.Args[i+1]
		}
	}
	if dir == "" {
		return fmt.Errorf("missing synthetic output")
	}
	if err := os.WriteFile(filepath.Join(dir, "partial.part"), []byte("synthetic partial download"), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(dir), "started.txt"), []byte(dir), 0600); err != nil {
		return err
	}
	time.Sleep(60 * time.Second)
	return fmt.Errorf("synthetic downloader was not cancelled")
}

func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	program, err := filepath.Abs(*binary)
	if err != nil {
		return err
	}
	parent := filepath.Join(root, ".local/ctb-cancel")
	if err = os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	out, err := os.MkdirTemp(parent, "run-")
	if err != nil {
		return err
	}
	h, err := qa.New(root, program, out)
	if err != nil {
		return err
	}
	defer h.Close()
	cfg := ctb.Defaults()
	cfg.Enabled = true
	cfg.Upload = true
	cfg.GroupAccess = "all"
	cfg.GroupDailyLimit = 1
	cfg.Executable, err = os.Executable()
	if err != nil {
		return err
	}
	cfg.OutputDir = filepath.Join(out, "downloads")
	configPath := filepath.Join(out, ".local/ctb/config.json")
	if err = os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	if err = qa.WriteJSON(configPath, cfg); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	bot, err := h.Start(ctx, "ctb-cancel")
	if err != nil {
		return err
	}
	defer bot.Stop()
	c := qa.NewClient(h.Address)
	if _, err = c.Auth(h.AdminDir); err != nil {
		return err
	}
	plugins, err := c.Checked("GET", "/api/plugins", nil)
	if err != nil {
		return err
	}
	for _, raw := range plugins["plugins"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "ctb" {
			_, err = c.Checked("POST", "/api/plugin-switch", map[string]any{"name": "ctb", "groupId": "0", "mode": "enabled", "revision": p["state"].(map[string]any)["revision"]})
			if err != nil {
				return err
			}
		}
	}
	groupSend := func(uid, gid int64, text string) (string, error) {
		return h.SendGroupAs(ctx, uid, gid, text, "admin", map[string]any{"type": "at", "data": map[string]string{"qq": "87654321"}}, map[string]any{"type": "text", "data": map[string]string{"text": text}})
	}
	text, err := groupSend(qa.Owner, 101, "FTB下载 135 latest full")
	if err != nil || !strings.Contains(text, "准备下载") {
		return fmt.Errorf("download not started: %s %v", text, err)
	}
	var started []byte
	deadline := time.Now().Add(10 * time.Second)
	for {
		started, err = os.ReadFile(filepath.Join(cfg.OutputDir, "started.txt"))
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("synthetic process did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	quotaPath := filepath.Join(out, ".local/ctb/group-usage.json")
	quota, err := os.ReadFile(quotaPath)
	if err != nil {
		return err
	}
	text, err = groupSend(qa.Owner+1, 101, "FTB取消")
	if err != nil || !strings.Contains(text, "仅限") {
		return fmt.Errorf("group admin cancellation allowed: %s %v", text, err)
	}
	text, err = groupSend(qa.Owner, 102, "FTB取消")
	if err != nil || !strings.Contains(text, "不来自本群") {
		return fmt.Errorf("cross-group cancellation allowed: %s %v", text, err)
	}
	// The acknowledgement and the original task's exit message may race; require both.
	first, _, err := h.SendWithContext(ctx, qa.Owner, "FTB取消")
	if err != nil {
		return err
	}
	second, err := h.AwaitReply(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(first+second, "已请求取消") || !strings.Contains(first+second, "任务已取消") {
		return fmt.Errorf("missing cancellation acknowledgement/completion: %s %s", first, second)
	}
	text, _, err = h.SendWithContext(ctx, qa.Owner, "FTB工具状态")
	if err != nil || !strings.Contains(text, "忙碌：false") {
		return fmt.Errorf("busy state retained: %s %v", text, err)
	}
	text, _, err = h.SendWithContext(ctx, qa.Owner, "FTB取消")
	if err != nil || !strings.Contains(text, "没有可取消") {
		return fmt.Errorf("idle cancel not reported: %s %v", text, err)
	}
	text, _, err = h.SendWithContext(ctx, qa.Owner, "FTB查询 135")
	if err != nil || !strings.Contains(text, "正在查询") {
		return fmt.Errorf("next query not started: %s %v", text, err)
	}
	text, err = h.AwaitReply(ctx)
	if err != nil || !strings.Contains(text, "synthetic query after cancellation") {
		return fmt.Errorf("next task did not recover: %s %v", text, err)
	}
	after, err := os.ReadFile(quotaPath)
	if err != nil || !bytes.Equal(quota, after) {
		return fmt.Errorf("cancel refunded or changed quota: %v", err)
	}
	if _, err = os.Stat(filepath.Join(string(started), "partial.part")); err != nil {
		return fmt.Errorf("partial file removed: %w", err)
	}
	if h.ActionCount("upload_group_file") != 0 || h.ActionCount("upload_private_file") != 0 {
		return fmt.Errorf("cancelled download uploaded files")
	}
	data, err := os.ReadFile(program)
	if err != nil {
		return err
	}
	result := map[string]any{"passed": true, "isolated": true, "realQQMessagesSent": false, "realModelCalls": 0, "realDownloads": 0, "binarySHA256": fmt.Sprintf("%x", sha256.Sum256(data)), "ownerOnly": true, "sourceGroupBoundary": true, "privateOwnerCancel": true, "acknowledgementAndCompletion": true, "busyReleased": true, "nextTaskRecovered": true, "partialFilePreserved": true, "quotaUnchanged": true, "noFileUpload": true}
	if err = qa.WriteJSON(filepath.Join(parent, "integration-result.json"), result); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(b))
	return nil
}
