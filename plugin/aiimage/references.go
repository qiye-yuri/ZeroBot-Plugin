package aiimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxReferenceBytes = 8 << 20
const maxReferenceTotal = 32 << 20
const maxReferenceCount = 16

type referenceSource struct{ URL, File string }
type referenceImage struct {
	DataURL              string
	Bytes, Width, Height int
	SHA256               string
}

func editEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || !validImageURL(raw) || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("参考图需要有效的OpenAI兼容图片接口地址")
	}
	if strings.HasSuffix(u.Path, "/images/generations") {
		u.Path = strings.TrimSuffix(u.Path, "/generations") + "/edits"
	} else if !strings.HasSuffix(u.Path, "/images/edits") {
		return "", fmt.Errorf("参考图需要以/images/generations结尾的接口地址，用于切换到同一服务的/images/edits")
	}
	return u.String(), nil
}
func loadReferenceBytes(ctx context.Context, client *http.Client, source string) ([]byte, error) {
	var encoded string
	switch {
	case strings.HasPrefix(source, "base64://"):
		encoded = strings.TrimPrefix(source, "base64://")
	case strings.HasPrefix(source, "data:image/"):
		header, tail, ok := strings.Cut(source, ",")
		if !ok || !strings.HasSuffix(header, ";base64") {
			return nil, fmt.Errorf("参考图内嵌数据格式不支持")
		}
		encoded = tail
	default:
		if !validImageURL(source) {
			return nil, fmt.Errorf("参考图缺少可读取的图片链接")
		}
		fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, source, nil)
		if err != nil {
			return nil, fmt.Errorf("参考图地址无效")
		}
		// No model credential is ever sent to attachment servers.
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("参考图下载失败或超时，请重新发送原图")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("参考图链接失效或无法访问，请重新发送原图")
		}
		if resp.ContentLength > maxReferenceBytes {
			return nil, fmt.Errorf("单张参考图超过8MiB")
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxReferenceBytes+1))
		if err != nil {
			return nil, fmt.Errorf("参考图读取失败")
		}
		if len(data) == 0 || len(data) > maxReferenceBytes {
			return nil, fmt.Errorf("参考图为空或超过8MiB")
		}
		return data, nil
	}
	if len(encoded) > base64.StdEncoding.EncodedLen(maxReferenceBytes) {
		return nil, fmt.Errorf("单张参考图超过8MiB")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) == 0 || len(data) > maxReferenceBytes {
		return nil, fmt.Errorf("参考图Base64无效或超过8MiB")
	}
	return data, nil
}
func prepareReferences(ctx context.Context, client *http.Client, sources []referenceSource, resolve func(string) referenceSource) ([]referenceImage, error) {
	if len(sources) > maxReferenceCount {
		return nil, fmt.Errorf("每条最多附带16张参考图")
	}
	if len(sources) == 0 {
		return nil, nil
	}
	overall, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	refs := make([]referenceImage, 0, len(sources))
	total := 0
	for i, source := range sources {
		if err := overall.Err(); err != nil {
			return nil, fmt.Errorf("参考图读取超时")
		}
		var data []byte
		var err error
		candidates := []string{source.URL, source.File}
		seen := map[string]bool{}
		for _, candidate := range candidates {
			if candidate == "" || seen[candidate] {
				continue
			}
			seen[candidate] = true
			data, err = loadReferenceBytes(overall, client, candidate)
			if err == nil {
				break
			}
		}
		if len(data) == 0 && resolve != nil && source.File != "" && !strings.ContainsAny(source.File, "/\\:") {
			resolved := resolve(source.File)
			for _, candidate := range []string{resolved.URL, resolved.File} {
				if candidate == "" || seen[candidate] {
					continue
				}
				data, err = loadReferenceBytes(overall, client, candidate)
				if err == nil {
					break
				}
			}
		}
		if len(data) == 0 {
			if err == nil {
				err = fmt.Errorf("没有可读取的图片数据")
			}
			return nil, fmt.Errorf("第%d张参考图无法读取：%w", i+1, err)
		}
		total += len(data)
		if total > maxReferenceTotal {
			return nil, fmt.Errorf("参考图合计超过32MiB，请减少张数或压缩图片")
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || (format != "png" && format != "jpeg" && format != "webp") {
			return nil, fmt.Errorf("第%d张参考图不是有效PNG/JPEG/WebP", i+1)
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || cfg.Width > (16<<20)/cfg.Height {
			return nil, fmt.Errorf("第%d张参考图尺寸过大（单边8192、总像素16M以内）", i+1)
		}
		if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
			return nil, fmt.Errorf("第%d张参考图文件损坏", i+1)
		}
		digest := sha256.Sum256(data)
		refs = append(refs, referenceImage{DataURL: "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(data), Bytes: len(data), Width: cfg.Width, Height: cfg.Height, SHA256: fmt.Sprintf("%x", digest)})
	}
	return refs, nil
}
func referenceBudget(c imageConfig, mode string, v renderSettings, refs []referenceImage) (int64, error) {
	if len(refs) == 0 {
		return 0, nil
	}
	if mode != "openai" {
		return 0, fmt.Errorf("带图绘制只支持OpenAI兼容编辑接口；不能忽略图片继续文生图")
	}
	if _, err := editEndpoint(c.APIURL); err != nil {
		return 0, err
	}
	if len(refs) > maxReferenceCount {
		return 0, fmt.Errorf("参考图超过16张")
	}
	for _, ref := range refs {
		if ref.Bytes <= 0 || ref.Bytes > maxReferenceBytes || ref.Width <= 0 || ref.Height <= 0 || !strings.HasPrefix(ref.DataURL, "data:image/") {
			return 0, fmt.Errorf("参考图尚未成功校验")
		}
	}
	if !officialEndpoint(c.APIURL) {
		return 0, nil
	} // Custom per-output ceiling must already cover its input images.
	o, err := optionsFor(v)
	if err != nil {
		return 0, err
	}
	reserve, err := parseDollars(o.ReferenceReserveUSD, 100*usd)
	if err != nil || reserve < 10000 {
		return 0, fmt.Errorf("每张参考图额外预留须为0.01–100美元")
	}
	// Policy allocation, not a provider token quote; input token sizing varies.
	return reserve * int64(len(refs)) * int64(o.Count), nil
}

// Only metadata returned by the trusted OneBot get_image call may reach here.
// Never read an arbitrary local path supplied by a message segment.
var qqUUIDImageName = regexp.MustCompile(`(?i)^\{([0-9a-f]{8})-([0-9a-f]{4})-([0-9a-f]{4})-([0-9a-f]{4})-([0-9a-f]{12})\}(\.(?:png|jpg|jpeg|webp))$`)

func sameQQImageName(cache, file string) bool {
	if strings.ContainsAny(file, `/\:`) {
		return false
	}
	if strings.EqualFold(cache, file) {
		return true
	}
	parts := qqUUIDImageName.FindStringSubmatch(file)
	return len(parts) == 7 && strings.EqualFold(cache, strings.Join(parts[1:6], "")+parts[6])
}

func oneBotReferenceSource(url, path, file string, bot int64) referenceSource {
	fallback := referenceSource{URL: url}
	if !filepath.IsAbs(path) || !sameQQImageName(filepath.Base(path), file) {
		return fallback
	}
	full, err := filepath.Abs(path)
	if err != nil {
		return fallback
	}
	marker := "/" + strconv.FormatInt(bot, 10) + "/nt_qq/nt_data/pic/"
	normalized := strings.ToLower(filepath.ToSlash(full))
	index := strings.Index(normalized, marker)
	if index < 0 {
		return fallback
	}
	root := full[:index+len(marker)-1]
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return fallback
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return fallback
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fallback
	}
	f, err := os.Open(resolved)
	if err != nil {
		return fallback
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxReferenceBytes {
		return fallback
	}
	data, err := io.ReadAll(io.LimitReader(f, maxReferenceBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxReferenceBytes {
		return fallback
	}
	return referenceSource{URL: url, File: "base64://" + base64.StdEncoding.EncodeToString(data)}
}
