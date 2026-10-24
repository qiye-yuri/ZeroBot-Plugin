package ctb

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func windowsArchivePath(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	if name == "" || strings.HasPrefix(name, "/") || !utf8.ValidString(name) {
		return "", fmt.Errorf("非法压缩包路径")
	}
	parts := strings.Split(name, "/")
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if p == "." || p == ".." || strings.TrimRight(p, ". ") != p {
			return "", fmt.Errorf("不安全的压缩包路径：%s", name)
		}
		for _, r := range p {
			if r < 32 || strings.ContainsRune(`<>:"|?*`, r) {
				return "", fmt.Errorf("Windows不支持的压缩包路径：%s", name)
			}
		}
		base := strings.ToUpper(strings.SplitN(p, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || (len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
			return "", fmt.Errorf("Windows保留文件名：%s", name)
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return "", fmt.Errorf("空压缩包路径")
	}
	result := strings.Join(clean, "/")
	if strings.HasSuffix(name, "/") {
		result += "/"
	}
	return result, nil
}

// PrepareArchive validates payload CRCs and Windows paths before delivery.
// Only redundant separators are repaired. Original archives and file contents
// are retained; traversal, reserved names and normalization collisions fail closed.
func PrepareArchive(input string) (output string, changes int, err error) {
	z, err := zip.OpenReader(input)
	if err != nil {
		return "", 0, fmt.Errorf("下载zip无法读取：%w", err)
	}
	defer z.Close()
	names := make([]string, len(z.File))
	seen := map[string]bool{}
	for i, f := range z.File {
		name, e := windowsArchivePath(f.Name)
		if e != nil {
			return "", 0, e
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return "", 0, fmt.Errorf("不支持压缩包符号链接：%s", name)
		}
		key := strings.ToLower(strings.TrimSuffix(name, "/"))
		if _, exists := seen[key]; exists {
			return "", 0, fmt.Errorf("压缩包路径规范化后冲突：%s", name)
		}
		seen[key] = strings.HasSuffix(name, "/")
		names[i] = name
		if name != f.Name {
			changes++
		}
		rd, e := f.Open()
		if e != nil {
			return "", 0, e
		}
		_, e = io.Copy(io.Discard, rd)
		rd.Close()
		if e != nil {
			return "", 0, fmt.Errorf("下载zip校验失败：%w", e)
		}
	}
	for key := range seen {
		parts := strings.Split(key, "/")
		for i := 1; i < len(parts); i++ {
			if dir, exists := seen[strings.Join(parts[:i], "/")]; exists && !dir {
				return "", 0, fmt.Errorf("压缩包文件与目录路径冲突：%s", key)
			}
		}
	}
	if changes == 0 {
		return input, 0, nil
	}
	output = strings.TrimSuffix(input, filepath.Ext(input)) + "-windows.zip"
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", 0, err
	}
	completed := false
	createdPath := output
	defer func() {
		f.Close()
		if !completed {
			os.Remove(createdPath)
		}
	}()
	w := zip.NewWriter(f)
	if err = w.SetComment(z.Comment); err != nil {
		return "", 0, err
	}
	for i, entry := range z.File {
		copy := *entry
		copy.Name = names[i]
		if err = w.Copy(&copy); err != nil {
			w.Close()
			return "", 0, err
		}
	}
	if err = w.Close(); err != nil {
		return "", 0, err
	}
	if err = f.Close(); err != nil {
		return "", 0, err
	}
	completed = true
	return output, changes, nil
}
