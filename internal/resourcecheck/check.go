// Package resourcecheck inspects existing plugin manifests without fetching or changing files.
package resourcecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/webp"
)

type File struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Bytes  int64  `json:"bytes"`
}
type Group struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Manifest   string `json:"manifest"`
	Error      string `json:"error,omitempty"`
	Expected   int    `json:"expected"`
	Valid      int    `json:"valid"`
	Missing    int    `json:"missing"`
	Damaged    int    `json:"damaged"`
	Unreadable int    `json:"unreadable"`
	Bytes      int64  `json:"bytes"`
	Files      []File `json:"files"`
}
type Report struct {
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	Groups      []Group   `json:"groups"`
}
type Snapshot struct {
	Busy   bool    `json:"busy"`
	Report *Report `json:"report"`
}
type Manager struct {
	root string
	mu   sync.Mutex
	busy bool
	last *Report // Immutable after publication; no report file or resource writes.
}

func New(root string) *Manager { return &Manager{root: root} }
func (m *Manager) Read() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Snapshot{m.busy, m.last}
}
func (m *Manager) Check(ctx context.Context) (Snapshot, error) {
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return Snapshot{}, fmt.Errorf("资源检查正在进行")
	}
	m.busy = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.busy = false; m.mu.Unlock() }()
	r, err := Inspect(ctx, m.root)
	if err != nil {
		return Snapshot{}, err
	}
	m.mu.Lock()
	m.last = &r
	m.mu.Unlock()
	return Snapshot{Report: &r}, nil
}

// Inspect covers only the built-in Tarot and Wife namespaces, never nwife or other plugins.
func Inspect(ctx context.Context, root string) (Report, error) {
	r := Report{StartedAt: time.Now().UTC()}
	for _, spec := range []Group{{ID: "tarot", Name: "塔罗固定牌图", Manifest: "Tarot/tarots.json"}, {ID: "wife", Name: "内置角色图片", Manifest: "Wife/wife.json"}} {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		g := spec
		g.Files = []File{}
		data, _, err := readLocal(root, g.Manifest, 4<<20)
		if err != nil {
			g.Error = "资源清单" + classify(err) + "：" + readReason(err)
			r.Groups = append(r.Groups, g)
			continue
		}
		names, err := manifestNames(g.ID, data)
		if err != nil {
			g.Error = "资源清单无效：" + err.Error()
			r.Groups = append(r.Groups, g)
			continue
		}
		g.Expected = len(names)
		for _, name := range names {
			if err := ctx.Err(); err != nil {
				return Report{}, err
			}
			data, size, err := readLocal(root, name, 8<<20)
			f := File{Path: name, Bytes: size, Status: "正常"}
			if err != nil {
				f.Status = classify(err)
				f.Reason = readReason(err)
			} else if exceedsPixelLimit(data) {
				f.Status = "无法读取"
				f.Reason = "像素数超过4000万检查上限，未完整解码，不代表文件损坏"
			} else if !validImage(data) {
				f.Status = "损坏"
				f.Reason = "图像解码失败或格式不受支持，需人工核对"
			}
			switch f.Status {
			case "正常":
				g.Valid++
			case "缺失":
				g.Missing++
			case "损坏":
				g.Damaged++
			default:
				g.Unreadable++
			}
			g.Bytes += size
			g.Files = append(g.Files, f)
		}
		r.Groups = append(r.Groups, g)
	}
	r.CompletedAt = time.Now().UTC()
	return r, nil
}
func safeRelative(name string) bool {
	return name != "" && !strings.ContainsAny(name, "\\:\x00") && !strings.HasPrefix(name, "/") && filepath.ToSlash(filepath.Clean(name)) == name && name != "." && name != ".." && !strings.HasPrefix(name, "../")
}
func manifestNames(kind string, data []byte) ([]string, error) {
	set := map[string]bool{}
	add := func(prefix, name string) error {
		if !safeRelative(name) {
			return fmt.Errorf("包含无效相对路径")
		}
		set[prefix+name] = true
		return nil
	}
	if kind == "tarot" {
		var cards map[string]struct {
			Info struct {
				ImgURL string `json:"imgUrl"`
			} `json:"info"`
		}
		if json.Unmarshal(data, &cards) != nil || len(cards) == 0 {
			return nil, fmt.Errorf("塔罗列表为空或格式错误")
		}
		for _, c := range cards {
			if err := add("Tarot/", c.Info.ImgURL); err != nil {
				return nil, err
			}
			if err := add("Tarot/Reverse/", c.Info.ImgURL); err != nil {
				return nil, err
			}
		}
	} else {
		var cards []string
		if json.Unmarshal(data, &cards) != nil || len(cards) == 0 {
			return nil, fmt.Errorf("角色列表为空或格式错误")
		}
		for _, c := range cards {
			if err := add("Wife/wives/", c); err != nil {
				return nil, err
			}
		}
	}
	if len(set) > 10000 {
		return nil, fmt.Errorf("资源数量超过检查上限")
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}
func classify(err error) string {
	if os.IsNotExist(err) {
		return "缺失"
	}
	return "无法读取"
}

var (
	errPath    = errors.New("invalid path")
	errLink    = errors.New("linked resource")
	errRegular = errors.New("not regular")
	errSize    = errors.New("size limit")
)

// Return bounded public reasons, never OS errors containing absolute paths.
func readReason(err error) string {
	switch {
	case os.IsNotExist(err):
		return "文件或上级目录不存在"
	case errors.Is(err, errPath):
		return "路径不在允许范围"
	case errors.Is(err, errLink):
		return "路径包含链接，未跟随读取"
	case errors.Is(err, errRegular):
		return "目标不是普通文件"
	case errors.Is(err, errSize):
		return "文件超过读取大小上限（清单4 MiB，图片8 MiB）"
	case os.IsPermission(err):
		return "没有读取权限"
	default:
		return "读取失败，请检查文件权限或占用情况"
	}
}
func readLocal(root, name string, limit int64) ([]byte, int64, error) {
	if !safeRelative(name) {
		return nil, 0, errPath
	}
	// Reject links/junctions below the selected root, including the root itself.
	p := root
	for _, part := range append([]string{""}, strings.Split(name, "/")...) {
		if part != "" {
			p = filepath.Join(p, part)
		}
		info, err := os.Lstat(p)
		if err != nil {
			return nil, 0, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, 0, errLink
		}
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, errRegular
	}
	if info.Size() > limit {
		return nil, info.Size(), errSize
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, info.Size(), err
	}
	if int64(len(b)) > limit {
		return nil, info.Size(), errSize
	}
	return b, info.Size(), nil
}
func exceedsPixelLimit(data []byte) bool {
	c, _, err := image.DecodeConfig(bytes.NewReader(data))
	return err == nil && int64(c.Width)*int64(c.Height) > 40_000_000
}
func validImage(data []byte) bool {
	c, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || c.Width <= 0 || c.Height <= 0 || int64(c.Width)*int64(c.Height) > 40_000_000 {
		return false
	}
	_, _, err = image.Decode(bytes.NewReader(data))
	return err == nil
}
