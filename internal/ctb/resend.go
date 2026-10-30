package ctb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type deliveryFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

var deliveryTaskID = regexp.MustCompile(`^ctb-[1-9][0-9]{0,9}-[0-9]+$`)

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Record only the final, validated archives, including repaired variants.
func recordDelivery(dir string, files []string) error {
	entries := make([]deliveryFile, 0, len(files))
	for _, file := range files {
		hash, err := fileDigest(file)
		if err != nil {
			return err
		}
		entries = append(entries, deliveryFile{filepath.Base(file), hash})
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "delivery.json"), b, 0600)
}

func deliveryFiles(root, task string) ([]string, error) {
	if !deliveryTaskID.MatchString(task) {
		return nil, fmt.Errorf("重发任务编号无效，请复制下载完成消息中的编号")
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("下载目录必须为绝对路径")
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("下载目录不可读取")
	}
	dir := filepath.Join(root, task)
	real, err := filepath.EvalSymlinks(dir)
	if err != nil || real != dir {
		return nil, fmt.Errorf("任务目录不存在或已被替换")
	}
	manifest := filepath.Join(dir, "delivery.json")
	if st, err := os.Lstat(manifest); err != nil || !st.Mode().IsRegular() {
		return nil, fmt.Errorf("没有可用重发记录；仅支持本版本完成的下载")
	}
	b, err := os.ReadFile(manifest)
	var entries []deliveryFile
	if err != nil || json.Unmarshal(b, &entries) != nil || len(entries) == 0 {
		return nil, fmt.Errorf("重发记录不可读取")
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if filepath.Base(entry.Name) != entry.Name || strings.ContainsAny(entry.Name, `/\:`) || !strings.EqualFold(filepath.Ext(entry.Name), ".zip") {
			return nil, fmt.Errorf("重发记录包含无效文件名")
		}
		p := filepath.Join(dir, entry.Name)
		if st, err := os.Lstat(p); err != nil || !st.Mode().IsRegular() {
			return nil, fmt.Errorf("已下载文件缺失或已被替换：%s", entry.Name)
		}
		hash, err := fileDigest(p)
		if err != nil || hash != entry.SHA256 {
			return nil, fmt.Errorf("文件与下载校验结果不一致：%s", entry.Name)
		}
		files = append(files, p)
	}
	return files, nil
}

// Resend shares the existing task lock, but never starts the downloader or charges quota.
// The QQ command authorizes the owner before entering this method.
func (s *Service) Resend(task string, requester, group int64, send func(string, map[string]any) error) error {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return fmt.Errorf("已有任务执行中，请稍后重试")
	}
	c, err := s.read()
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("配置读取失败")
	}
	if !c.Enabled || !c.Upload {
		s.mu.Unlock()
		return fmt.Errorf("请先启用FTB工具及文件发送")
	}
	if !CanUse(group > 0, true, true, true, c.GroupAccess, "resend") {
		s.mu.Unlock()
		return fmt.Errorf("当前群聊权限不允许重发")
	}
	s.busy = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.busy = false; s.mu.Unlock() }()
	files, err := deliveryFiles(c.OutputDir, task)
	if err != nil {
		return err
	}
	return Deliver(files, requester, group, c.GroupDelivery, send)
}
