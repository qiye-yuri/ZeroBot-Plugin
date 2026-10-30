//go:build ignore

// Synthetic downloader and OneBot only; no FTB network or real QQ sends.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	ctb "github.com/FloatTech/ZeroBot-Plugin/internal/ctb"
)

var binary = flag.String("binary", ".local/bin/zerobot-ctb-resend.exe", "candidate executable")

func main() {
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
		return fmt.Errorf("missing synthetic output directory")
	}
	f, err := os.Create(filepath.Join(dir, "fixture.zip"))
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	w, err := z.Create("overrides//synthetic.txt")
	if err != nil {
		return err
	}
	if _, err = w.Write([]byte("synthetic download, no network")); err != nil {
		return err
	}
	if err = z.Close(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	calls, err := os.OpenFile(filepath.Join(filepath.Dir(dir), "downloader-calls.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer calls.Close()
	_, err = calls.WriteString("download\n")
	return err
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
	parent := filepath.Join(root, ".local/ctb-resend")
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
	bot, err := h.Start(ctx, "ctb-resend")
	if err != nil {
		return err
	}
	defer func() { bot.Stop() }()
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
	groupSend := func(uid int64, text string) (string, error) {
		return h.SendGroupAs(ctx, uid, 101, text, "member", map[string]any{"type": "at", "data": map[string]string{"qq": "87654321"}}, map[string]any{"type": "text", "data": map[string]string{"text": text}})
	}
	h.FailAction("upload_group_file")
	text, err := groupSend(qa.Owner, "FTB下载 135 latest full")
	if err != nil || !strings.Contains(text, "准备下载") {
		return fmt.Errorf("download not started: %s %v", text, err)
	}
	text, err = h.AwaitReply(ctx)
	if err != nil {
		return err
	}
	task := regexp.MustCompile(`ctb-135-[0-9]+`).FindString(text)
	if task == "" {
		return fmt.Errorf("missing resend task: %s", text)
	}
	text, err = h.AwaitReply(ctx)
	if err != nil || !strings.Contains(text, "FTB重发 "+task) {
		return fmt.Errorf("missing actionable failure: %s %v", text, err)
	}
	quotaPath := filepath.Join(out, ".local/ctb/group-usage.json")
	quota, err := os.ReadFile(quotaPath)
	if err != nil {
		return err
	}
	manifest, err := os.ReadFile(filepath.Join(cfg.OutputDir, task, "delivery.json"))
	if err != nil {
		return err
	}
	if !bytes.Contains(manifest, []byte("fixture-windows.zip")) || bytes.Contains(manifest, []byte(`"fixture.zip"`)) {
		return fmt.Errorf("receipt did not select repaired archive")
	}
	before := h.ActionCount("upload_group_file")
	text, err = groupSend(qa.Owner+1, "FTB重发 "+task)
	if err != nil || !strings.Contains(text, "仅限") {
		return fmt.Errorf("member resend allowed: %s %v", text, err)
	}
	if h.ActionCount("upload_group_file") != before {
		return fmt.Errorf("unauthorized upload")
	}
	h.FailAction("")
	text, err = groupSend(qa.Owner, "FTB重发 "+task)
	if err != nil || !strings.Contains(text, "已接受重发") {
		return fmt.Errorf("group resend failed: %s %v", text, err)
	}
	if h.ActionCount("upload_group_file") != before+1 {
		return fmt.Errorf("wrong group resend call count")
	}
	bot.Stop()
	bot, err = h.Start(ctx, "ctb-resend-restart")
	if err != nil {
		return err
	}
	privateBefore := h.ActionCount("upload_private_file")
	text, _, err = h.SendWithContext(ctx, qa.Owner, "FTB重发 "+task)
	if err != nil || !strings.Contains(text, "已接受重发") {
		return fmt.Errorf("restart private resend failed: %s %v", text, err)
	}
	if h.ActionCount("upload_private_file") != privateBefore+1 {
		return fmt.Errorf("private resend missing")
	}
	if err = os.WriteFile(filepath.Join(cfg.OutputDir, task, "fixture-windows.zip"), []byte("changed"), 0600); err != nil {
		return err
	}
	text, _, err = h.SendWithContext(ctx, qa.Owner, "FTB重发 "+task)
	if err != nil || !strings.Contains(text, "不一致") {
		return fmt.Errorf("changed file accepted: %s %v", text, err)
	}
	if h.ActionCount("upload_private_file") != privateBefore+1 {
		return fmt.Errorf("changed file uploaded")
	}
	after, err := os.ReadFile(quotaPath)
	if err != nil || !bytes.Equal(quota, after) {
		return fmt.Errorf("resend altered group quota: %v", err)
	}
	calls, err := os.ReadFile(filepath.Join(cfg.OutputDir, "downloader-calls.txt"))
	if err != nil || string(calls) != "download\n" {
		return fmt.Errorf("resend invoked downloader: %q %v", calls, err)
	}
	data, err := os.ReadFile(program)
	if err != nil {
		return err
	}
	result := map[string]any{"passed": true, "isolated": true, "realQQMessagesSent": false, "realModelCalls": 0, "realDownloads": 0, "binarySHA256": fmt.Sprintf("%x", sha256.Sum256(data)), "failedSendProvidesTask": true, "ownerOnly": true, "groupResend": true, "privateResendAfterRestart": true, "repairedFileSelected": true, "changedFileRejected": true, "quotaUnchanged": true, "downloaderCalls": 1}
	if err = qa.WriteJSON(filepath.Join(parent, "integration-result.json"), result); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(b))
	return nil
}
