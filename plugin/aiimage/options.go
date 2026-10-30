package aiimage

import (
	"encoding/json"
	"fmt"
	core "github.com/FloatTech/ZeroBot-Plugin/internal/imagegen"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

func defaultOptions() core.GenerationOptions {
	return core.GenerationOptions{ReferenceReserveUSD: "0.25", Background: "auto", Format: "png", Compression: 100, Count: 1, Moderation: "auto", TimeoutSeconds: 300}
}
func optionsFor(v renderSettings) (core.GenerationOptions, error) {
	o := defaultOptions()
	if v.OptionsJSON != "" {
		if err := json.Unmarshal([]byte(v.OptionsJSON), &o); err != nil {
			return o, fmt.Errorf("画图扩展配置损坏")
		}
	}
	if o.ReferenceReserveUSD == "" {
		o.ReferenceReserveUSD = "0.25"
	}
	return o, nil
}
func putOptions(v *renderSettings, o core.GenerationOptions) {
	b, _ := json.Marshal(o)
	v.OptionsJSON = string(b)
}
func normalizeSize(size string) string {
	size = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(size)), "×", "x")
	switch size {
	case "1k":
		return "1024x1024"
	case "2k":
		return "2048x2048"
	case "2k-landscape":
		return "2048x1152"
	case "2k-portrait":
		return "1152x2048"
	case "4k", "4k-landscape":
		return "3840x2160"
	case "4k-portrait":
		return "2160x3840"
	}
	return size
}

var dimensionsRE = regexp.MustCompile(`^([1-9][0-9]{2,3})x([1-9][0-9]{2,3})$`)

func dimensions(size string) (int64, int64, error) {
	m := dimensionsRE.FindStringSubmatch(size)
	if m == nil {
		return 0, 0, fmt.Errorf("尺寸格式须为宽x高")
	}
	w, _ := strconv.ParseInt(m[1], 10, 64)
	h, _ := strconv.ParseInt(m[2], 10, 64)
	if w > 3840 || h > 3840 || w%16 != 0 || h%16 != 0 || w*h < 655360 || w*h > 8294400 || w > 3*h || h > 3*w {
		return 0, 0, fmt.Errorf("尺寸须为16的倍数、单边不超过3840、比例不超过3:1、总像素655360至8294400；4K横图用3840x2160")
	}
	return w, h, nil
}
func validateOptions(c imageConfig, mode string, v renderSettings) error {
	if !validQuality(v.Quality) || !validSize(v.Size) {
		return fmt.Errorf("质量或尺寸不受支持")
	}
	o, err := optionsFor(v)
	if err != nil {
		return err
	}
	if amount, e := parseDollars(o.ReferenceReserveUSD, 100*usd); e != nil || amount < 10000 {
		return fmt.Errorf("每张参考图额外预留须为0.01–100美元")
	}
	if o.Background != "auto" && o.Background != "opaque" && o.Background != "transparent" {
		return fmt.Errorf("背景须为auto、opaque或transparent")
	}
	if o.Format != "png" && o.Format != "jpeg" && o.Format != "webp" {
		return fmt.Errorf("输出格式须为png、jpeg或webp")
	}
	if o.Background == "transparent" && o.Format == "jpeg" {
		return fmt.Errorf("透明背景不支持JPEG，请选PNG或WebP")
	}
	if o.Compression < 0 || o.Compression > 100 || o.Count < 1 || o.Count > 10 || o.PartialImages < 0 || o.PartialImages > 3 {
		return fmt.Errorf("压缩率须为0–100，图片张数1–10，预览张数0–3")
	}
	if !o.Stream && o.PartialImages != 0 {
		return fmt.Errorf("流式预览须先启用流式生成")
	}
	if o.Moderation != "auto" && o.Moderation != "low" {
		return fmt.Errorf("审核级别须为auto或low")
	}
	if len(o.SafetyIdentifier) > 128 || strings.ContainsAny(o.SafetyIdentifier, "\r\n") {
		return fmt.Errorf("安全标识最多128字节且不能含换行")
	}
	if o.TimeoutSeconds < 30 || o.TimeoutSeconds > 1200 {
		return fmt.Errorf("等待上限须为30–1200秒")
	}
	if o.ProxyURL != "" {
		u, e := url.Parse(o.ProxyURL)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
			return fmt.Errorf("代理须为不含账号密码的HTTP/HTTPS/SOCKS5地址")
		}
	}
	family := modelFamily(c.ModelName)
	if mode == "openai" && (family == "gpt-image-1.5" || family == "gpt-image-1-mini") {
		if v.Quality == "xhigh" || v.Quality == "max" {
			return fmt.Errorf("该旧模型不支持xhigh/max")
		}
		if v.Size != "auto" && v.Size != "1024x1024" && v.Size != "1536x1024" && v.Size != "1024x1536" {
			return fmt.Errorf("该旧模型只支持三个标准尺寸或auto，不支持2K/4K")
		}
	}
	if mode == "openai" && family == "gpt-image-2" && (v.Quality == "xhigh" || v.Quality == "max") {
		return fmt.Errorf("GPT Image 2不支持xhigh/max")
	}
	if mode == "siliconflow" && (v.Size == "auto" || o.Background != "auto" || o.Format != "png" || o.Compression != 100 || o.Moderation != "auto" || o.Stream || o.PartialImages != 0 || o.SafetyIdentifier != "") {
		return fmt.Errorf("原硅基流动协议仅支持尺寸和张数；其他生成选项请使用OpenAI兼容协议")
	}
	return nil
}

var imageArgumentsRE = regexp.MustCompile(`(?:^|\s)--([a-z-]+)=([^\s]+)`)

func parseImageOptions(prompt string, v renderSettings) (string, renderSettings, error) {
	o, err := optionsFor(v)
	if err != nil {
		return "", v, err
	}
	seen := map[string]bool{}
	var parseErr error
	clean := imageArgumentsRE.ReplaceAllStringFunc(prompt, func(s string) string {
		m := imageArgumentsRE.FindStringSubmatch(s)
		name, value := m[1], m[2]
		if seen[name] {
			parseErr = fmt.Errorf("参数--%s重复", name)
			return " "
		}
		seen[name] = true
		integer := func() int {
			x, e := strconv.Atoi(value)
			if e != nil {
				parseErr = fmt.Errorf("--%s须为整数", name)
			}
			return x
		}
		switch name {
		case "size":
			v.Size = normalizeSize(value)
		case "quality":
			v.Quality = value
		case "background":
			o.Background = value
		case "format":
			o.Format = value
		case "compression":
			o.Compression = integer()
		case "n":
			o.Count = integer()
		case "moderation":
			o.Moderation = value
		case "stream":
			if value != "true" && value != "false" {
				parseErr = fmt.Errorf("--stream须为true或false")
			}
			o.Stream = value == "true"
		case "partial-images":
			o.PartialImages = integer()
		default:
			parseErr = fmt.Errorf("未知画图参数--%s；接口、模型、密钥和代理只能在后台设置", name)
		}
		return " "
	})
	if parseErr != nil {
		return "", v, parseErr
	}
	clean = strings.TrimSpace(clean)
	if clean == "" || len(clean) > 32000 {
		return "", v, fmt.Errorf("请填写图片描述（最多32000字节）")
	}
	if !validSize(v.Size) || !validQuality(v.Quality) {
		return "", v, fmt.Errorf("不支持的尺寸或质量，请检查--size与--quality")
	}
	putOptions(&v, o)
	return clean, v, nil
}
