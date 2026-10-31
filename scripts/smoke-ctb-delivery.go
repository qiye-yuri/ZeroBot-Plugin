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
	"sync"
	"time"

	qa "github.com/FloatTech/ZeroBot-Plugin/internal/adminqa"
	ctb "github.com/FloatTech/ZeroBot-Plugin/internal/ctb"
)

var binary = flag.String("binary", ".local/bin/zerobot-ctb-delivery-lifecycle.exe", "candidate executable")

func main() {
	if len(os.Args) > 1 && os.Args[1] == "inspect" {
		fmt.Println("query after upload")
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
	parent := filepath.Join(root, ".local/ctb-delivery-lifecycle")
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
	cfg.GroupDailyLimit = 2
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
	bot, err := h.Start(ctx, "ctb-delivery")
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
	groupSend := func(text string) (string, error) {
		return h.SendGroupAs(ctx, qa.Owner, 101, text, "owner", map[string]any{"type": "at", "data": map[string]string{"qq": "87654321"}}, map[string]any{"type": "text", "data": map[string]string{"text": text}})
	}
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	h.HoldFileUploads(release)
	reply, err := groupSend("FTB下载 135 latest full")
	if err != nil || !strings.Contains(reply, "准备下载") {
		return fmt.Errorf("download not started: %s %v", reply, err)
	}
	reply, err = h.AwaitReply(ctx)
	if err != nil {
		return err
	}
	task := regexp.MustCompile("ctb-135-[0-9]+").FindString(reply)
	if task == "" || !strings.Contains(reply, "正在发送文件") {
		return fmt.Errorf("missing upload phase: %s", reply)
	}
	deadline := time.Now().Add(5 * time.Second)
	for h.ActionCount("upload_group_file") < 1 {
		if time.Now().After(deadline) {
			return fmt.Errorf("upload not called")
		}
		time.Sleep(10 * time.Millisecond)
	}
	quotaPath := filepath.Join(out, ".local/ctb/group-usage.json")
	quota, err := os.ReadFile(quotaPath)
	if err != nil {
		return err
	}
	reply, err = groupSend("FTB工具状态")
	if err != nil || !strings.Contains(reply, "忙碌：true") {
		return fmt.Errorf("upload reported idle: %s %v", reply, err)
	}
	reply, err = groupSend("FTB重发 " + task)
	if err != nil || !strings.Contains(reply, "已有任务执行中") {
		return fmt.Errorf("overlapping resend allowed: %s %v", reply, err)
	}
	reply, err = groupSend("FTB下载 135 latest full")
	if err != nil {
		return err
	}
	reply, err = h.AwaitReply(ctx)
	if err != nil || !strings.Contains(reply, "已有任务执行中") {
		return fmt.Errorf("overlapping download allowed: %s %v", reply, err)
	}
	snapshot, err := c.Checked("GET", "/api/ctb", nil)
	if err != nil {
		return err
	}
	next := cfg
	next.Threads++
	status, _, _, err := c.Request("POST", "/api/ctb", map[string]any{"revision": snapshot["revision"], "config": next})
	if err != nil || status != 409 {
		return fmt.Errorf("configuration changed during upload: %d %v", status, err)
	}
	reply, err = groupSend("FTB取消")
	if err != nil || !strings.Contains(reply, "文件发送阶段") {
		return fmt.Errorf("upload cancel phase wrong: %s %v", reply, err)
	}
	after, err := os.ReadFile(quotaPath)
	if err != nil || !bytes.Equal(quota, after) || h.ActionCount("upload_group_file") != 1 {
		return fmt.Errorf("blocked operations caused download/upload or quota change")
	}
	releaseOnce.Do(func() { close(release) })
	h.HoldFileUploads(nil)
	reply, err = h.AwaitReply(ctx)
	if err != nil || !strings.Contains(reply, "QQ接口已接受文件发送") {
		return fmt.Errorf("success receipt missing: %s %v", reply, err)
	}
	reply, err = groupSend("FTB工具状态")
	if err != nil || !strings.Contains(reply, "忙碌：false") {
		return fmt.Errorf("success did not release busy: %s %v", reply, err)
	}
	h.FailAction("upload_group_file")
	reply, err = groupSend("FTB下载 135 latest full")
	if err != nil {
		return err
	}
	if _, err = h.AwaitReply(ctx); err != nil {
		return err
	}
	reply, err = h.AwaitReply(ctx)
	if err != nil || !strings.Contains(reply, "文件已保留本机") || !strings.Contains(reply, "FTB重发") {
		return fmt.Errorf("failure receipt lost files/instructions: %s %v", reply, err)
	}
	h.FailAction("")
	reply, err = groupSend("FTB工具状态")
	if err != nil || !strings.Contains(reply, "忙碌：false") {
		return fmt.Errorf("failure did not release busy: %s %v", reply, err)
	}
	reply, err = groupSend("FTB查询 135")
	if err != nil {
		return err
	}
	reply, err = h.AwaitReply(ctx)
	if err != nil || !strings.Contains(reply, "query after upload") {
		return fmt.Errorf("next task did not recover: %s %v", reply, err)
	}
	calls, err := os.ReadFile(filepath.Join(cfg.OutputDir, "downloader-calls.txt"))
	if err != nil || string(calls) != "download\ndownload\n" {
		return fmt.Errorf("unexpected downloader calls: %q %v", calls, err)
	}
	data, err := os.ReadFile(program)
	if err != nil {
		return err
	}
	result := map[string]any{"passed": true, "isolated": true, "realQQMessagesSent": false, "realDownloads": 0, "realModelCalls": 0, "binarySHA256": fmt.Sprintf("%x", sha256.Sum256(data)), "busyDuringInitialUpload": true, "overlappingRequestsRejected": true, "configSaveRejected": true, "cancelPhaseCorrect": true, "rejectedQuotaUnchanged": true, "successAndFailureReleaseBusy": true, "failureKeepsResendInstructions": true, "nextTaskRecovered": true}
	if err = qa.WriteJSON(filepath.Join(parent, "integration-result.json"), result); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(b))
	return nil
}
