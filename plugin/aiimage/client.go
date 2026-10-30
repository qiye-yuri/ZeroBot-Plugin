package aiimage

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var imageHTTPClient = &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}

const maxImageResponse = 256 << 20

func validImageURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil
}
func generateImage(client *http.Client, cfg imageConfig, mode, prompt string, options ...renderSettings) ([]string, error) {
	v := defaultSettings()
	if len(options) > 0 {
		v = options[0]
	}
	return generateImageWithPreview(client, cfg, mode, prompt, v, nil)
}
func generateImageWithPreview(client *http.Client, cfg imageConfig, mode, prompt string, v renderSettings, preview func(string, int), references ...referenceImage) ([]string, error) {
	if !validImageURL(cfg.APIURL) {
		return nil, fmt.Errorf("接口地址必须是完整的http或https URL")
	}
	if err := validateOptions(cfg, mode, v); err != nil {
		return nil, err
	}
	o, _ := optionsFor(v)
	payload := map[string]any{"model": cfg.ModelName, "prompt": prompt}
	switch mode {
	case "openai":
		payload["size"], payload["n"], payload["quality"] = v.Size, o.Count, v.Quality
		payload["background"], payload["output_format"], payload["moderation"] = o.Background, o.Format, o.Moderation
		if o.Format != "png" {
			payload["output_compression"] = o.Compression
		}
		if o.Stream {
			payload["stream"] = true
			payload["partial_images"] = o.PartialImages
		}
		if o.SafetyIdentifier != "" {
			payload["safety_identifier"] = o.SafetyIdentifier
		}
	case "siliconflow":
		payload["image_size"], payload["batch_size"] = v.Size, o.Count
		payload["num_inference_steps"], payload["guidance_scale"] = 20, 7.5
	default:
		return nil, fmt.Errorf("未知画图接口类型")
	}
	endpoint := cfg.APIURL
	if len(references) > 0 {
		if _, err := referenceBudget(cfg, mode, v, references); err != nil {
			return nil, err
		}
		var err error
		endpoint, err = editEndpoint(cfg.APIURL)
		if err != nil {
			return nil, err
		}
		inputs := make([]map[string]string, 0, len(references))
		for _, ref := range references {
			inputs = append(inputs, map[string]string{"image_url": ref.DataURL})
		}
		payload["images"] = inputs
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建画图请求失败")
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	current := *client
	current.Timeout = time.Duration(o.TimeoutSeconds) * time.Second
	if o.ProxyURL != "" {
		var transport *http.Transport
		if client.Transport == nil {
			transport = http.DefaultTransport.(*http.Transport).Clone()
		} else if t, ok := client.Transport.(*http.Transport); ok {
			transport = t.Clone()
		} else {
			return nil, fmt.Errorf("当前传输不支持代理")
		}
		proxy, _ := url.Parse(o.ProxyURL)
		transport.Proxy = http.ProxyURL(proxy)
		current.Transport = transport
		defer transport.CloseIdleConnections()
	}
	resp, err := current.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接画图接口或等待超时，请检查画图专用代理、网络与等待上限")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, imageAPIError(resp, cfg, prompt, references)
	}
	if o.Stream && strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return readImageStream(resp.Body, o.Count, o.PartialImages, preview)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageResponse+1))
	if err != nil {
		return nil, fmt.Errorf("读取图片响应失败")
	}
	if len(data) > maxImageResponse {
		return nil, fmt.Errorf("图片响应超过256MiB限制")
	}
	var result struct {
		Data   []imageItem     `json:"data"`
		Images []imageItem     `json:"images"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &result) != nil {
		return nil, fmt.Errorf("接口未返回有效的图片JSON")
	}
	if len(result.Error) > 0 && string(result.Error) != "null" {
		return nil, fmt.Errorf("服务商返回错误，请检查接口配置、模型权限与额度")
	}
	items := append(result.Data, result.Images...)
	if len(items) > o.Count {
		return nil, fmt.Errorf("接口返回图片数超过请求张数")
	}
	images := make([]string, 0, len(items))
	for _, item := range items {
		img, err := decodeImageItem(item)
		if err != nil {
			return nil, err
		}
		if img != "" {
			images = append(images, img)
		}
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("接口未返回可发送的图片（需要data[].b64_json或图片URL）")
	}
	return images, nil
}

type imageItem struct {
	URL    string `json:"url"`
	Base64 string `json:"b64_json"`
}

func decodeImageItem(item imageItem) (string, error) {
	if item.Base64 != "" {
		data, err := base64.StdEncoding.DecodeString(item.Base64)
		if err != nil || len(data) == 0 {
			return "", fmt.Errorf("接口返回了无效的Base64图片")
		}
		return "base64://" + base64.StdEncoding.EncodeToString(data), nil
	}
	if validImageURL(item.URL) {
		return item.URL, nil
	}
	return "", nil
}
func readImageStream(body io.Reader, count, partial int, preview func(string, int)) ([]string, error) {
	limited := &io.LimitedReader{R: body, N: maxImageResponse + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), maxImageResponse)
	finals := []string{}
	previewCount := 0
	seen := map[int]bool{}
	eventName := ""
	var data strings.Builder
	process := func() error {
		if data.Len() == 0 {
			return nil
		}
		raw := strings.TrimSuffix(data.String(), "\n")
		data.Reset()
		if raw == "[DONE]" {
			return nil
		}
		var event struct {
			Type   string          `json:"type"`
			Base64 string          `json:"b64_json"`
			Index  *int            `json:"image_index"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(raw), &event) != nil {
			return fmt.Errorf("流式接口返回无效JSON")
		}
		if event.Type == "" {
			event.Type = eventName
		}
		if event.Type == "error" || (len(event.Error) > 0 && string(event.Error) != "null") {
			return fmt.Errorf("流式生成失败，预览图不代表最终结果")
		}
		switch event.Type {
		case "image_generation.partial_image", "image_edit.partial_image":
			if previewCount >= count*partial {
				return fmt.Errorf("接口预览图数量超过请求上限")
			}
			img, err := decodeImageItem(imageItem{Base64: event.Base64})
			if err != nil || img == "" {
				return fmt.Errorf("无效的流式预览图")
			}
			previewCount++
			if preview != nil {
				preview(img, previewCount)
			}
		case "image_generation.completed", "image_edit.completed":
			if event.Index != nil {
				if *event.Index < 0 || *event.Index >= count || seen[*event.Index] {
					return fmt.Errorf("流式最终图片序号异常")
				}
				seen[*event.Index] = true
			}
			img, err := decodeImageItem(imageItem{Base64: event.Base64})
			if err != nil || img == "" {
				return fmt.Errorf("无效的流式最终图片")
			}
			finals = append(finals, img)
			if len(finals) > count {
				return fmt.Errorf("流式最终图片数量异常")
			}
		}
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := process(); err != nil {
				return nil, err
			}
			eventName = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		if strings.HasPrefix(line, "data:") {
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			data.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("流式响应读取失败")
	}
	if limited.N <= 0 {
		return nil, fmt.Errorf("流式响应超过256MiB限制")
	}
	if err := process(); err != nil {
		return nil, err
	}
	if len(finals) != count {
		return nil, fmt.Errorf("流式响应未完成：收到%d/%d张最终图片", len(finals), count)
	}
	return finals, nil
}
