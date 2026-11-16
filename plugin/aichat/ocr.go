package aichat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/FloatTech/zbputils/chat"
	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const ocrEndpoint = "https://api.deepseek.com/v1/tools/ocr"
const ocrImageLimit = 8 << 20

func ocrCommand(s string) bool {
	s = strings.TrimSpace(s)
	return strings.EqualFold(s, "OCR") || s == "图片转文字"
}
func ocrImageSource(ctx context.Context, event *zero.Event, call func(context.Context, string, zero.Params) zero.APIResponse) (string, error) {
	segments := message.ParseMessage(event.NativeMessage)
	images := []string{}
	replyID := ""
	collect := func(m message.Message) {
		for _, s := range m {
			if s.Type == "image" {
				source := s.Data["url"]
				if source == "" {
					source = s.Data["file"]
				}
				images = append(images, source)
			}
		}
	}
	collect(segments)
	for _, s := range segments {
		if s.Type == "reply" {
			if replyID != "" {
				return "", errors.New("请一次只引用一条图片消息。")
			}
			replyID = s.Data["id"]
		}
	}
	if len(images) == 0 && replyID != "" {
		id, e := strconv.ParseInt(replyID, 10, 64)
		if e != nil || id == 0 {
			return "", errors.New("引用消息编号无效。")
		}
		r := call(ctx, "get_msg", zero.Params{"message_id": id})
		if r.Status != "ok" || r.RetCode != 0 || r.Data.Get("message_id").Int() != id {
			return "", errors.New("无法读取引用消息，请重新发送图片。")
		}
		if event.GroupID != 0 {
			if r.Data.Get("group_id").Int() != event.GroupID {
				return "", errors.New("只能识别本群引用的图片。")
			}
		} else {
			if r.Data.Get("group_id").Int() != 0 || r.Data.Get("message_type").String() != "private" || r.Data.Get("sender.user_id").Int() != event.UserID {
				return "", errors.New("只能识别当前私聊用户发送的图片。")
			}
		}
		collect(message.ParseMessage([]byte(r.Data.Get("message").Raw)))
	}
	if len(images) != 1 {
		return "", errors.New("请随OCR命令发送一张图片，或引用一条只含一张图片的消息。")
	}
	source := images[0]
	if source != "" && !strings.ContainsAny(source, "/\\:") {
		r := call(ctx, "get_image", zero.Params{"file": source})
		if r.Status != "ok" || r.RetCode != 0 {
			return "", errors.New("图片信息读取失败，请重新发送原图。")
		}
		source = r.Data.Get("url").String()
	}
	return source, nil
}
func loadOCRImage(ctx context.Context, source string, client *http.Client) ([]byte, string, error) {
	var data []byte
	var err error
	if strings.HasPrefix(source, "base64://") {
		s := strings.TrimPrefix(source, "base64://")
		if len(s) > base64.StdEncoding.EncodedLen(ocrImageLimit) {
			return nil, "", errors.New("OCR图片超过8MiB。")
		}
		data, err = base64.StdEncoding.DecodeString(s)
	} else {
		u, e := url.Parse(source)
		if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || !(u.Hostname() == "multimedia.nt.qq.com" || u.Hostname() == "qpic.cn" || strings.HasSuffix(u.Hostname(), ".qpic.cn")) {
			return nil, "", errors.New("OCR仅读取QQ图片附件，不接受任意网址或本机文件。")
		}
		c := *client
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		req, e := http.NewRequestWithContext(fetchCtx, "GET", source, nil)
		if e != nil {
			return nil, "", errors.New("图片地址无效。")
		}
		r, e := c.Do(req)
		if e != nil {
			return nil, "", errors.New("图片下载失败或超时。")
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return nil, "", fmt.Errorf("图片下载失败（HTTP %d），请重新发送原图。", r.StatusCode)
		}
		if r.ContentLength > ocrImageLimit {
			return nil, "", errors.New("OCR图片超过8MiB。")
		}
		data, err = io.ReadAll(io.LimitReader(r.Body, ocrImageLimit+1))
	}
	if err != nil || len(data) == 0 || len(data) > ocrImageLimit {
		return nil, "", errors.New("图片为空、损坏或超过8MiB。")
	}
	cfg, format, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || !(format == "png" || format == "jpeg" || format == "webp") {
		return nil, "", errors.New("本版OCR支持PNG、JPEG和WebP图片。")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 8192 || cfg.Height > 8192 || cfg.Width > 16000000/cfg.Height {
		return nil, "", errors.New("图片尺寸过大，请缩小后重试。")
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return nil, "", errors.New("图片内容损坏或不完整，请重新发送原图。")
	}
	return data, "image." + format, nil
}

type ocrResult struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	PageCount int    `json:"page_count"`
	Pages     []struct {
		Index  int    `json:"index"`
		Text   string `json:"text"`
		Finish string `json:"finish_reason"`
	} `json:"pages"`
	Usage json.RawMessage `json:"usage"`
	Error json.RawMessage `json:"error"`
}

func requestOCR(ctx context.Context, client *http.Client, key chat.ModelKey, data []byte, name string) (ocrResult, error) {
	var result ocrResult
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, e := writer.CreateFormFile("file", name)
	if e != nil {
		return result, e
	}
	if _, e = file.Write(data); e != nil {
		return result, e
	}
	if e = writer.WriteField("purpose", "text_recognition"); e != nil {
		return result, e
	}
	if e = writer.Close(); e != nil {
		return result, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", ocrEndpoint, &body)
	if e != nil {
		return result, e
	}
	req.Header.Set("Authorization", "Bearer "+string(key))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, e := c.Do(req)
	if e != nil {
		return result, errors.New("OCR连接失败或超时；未自动重试，已提交的识别可能继续计费。")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return result, fmt.Errorf("OCR服务返回HTTP %d（请检查文件、余额或额度），未自动重试。", resp.StatusCode)
	}
	raw, e := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if e != nil || len(raw) > 2<<20 {
		return result, errors.New("OCR响应不完整或过大；未自动重试。")
	}
	if json.Unmarshal(raw, &result) != nil {
		return result, errors.New("OCR响应格式无效。")
	}
	if len(result.Error) > 0 && string(result.Error) != "null" {
		return result, errors.New("OCR识别失败，文件可能不受支持或服务超时；未自动重试。")
	}
	if result.Object != "ocr.result" || result.ID == "" || result.PageCount < 1 || result.PageCount > 1000 || len(result.Pages) != result.PageCount {
		return result, errors.New("OCR页数或结果格式无效。")
	}
	seen := map[int]bool{}
	for _, p := range result.Pages {
		if p.Index < 0 || p.Index >= result.PageCount || seen[p.Index] || (p.Finish != "stop" && p.Finish != "length") {
			return result, errors.New("OCR页面缺失、重复或状态无效。")
		}
		seen[p.Index] = true
	}
	return result, nil
}
func ocrReply(r ocrResult) string {
	var out strings.Builder
	fmt.Fprintf(&out, "OCR识别结果（%d页）：", r.PageCount)
	for i := 0; i < r.PageCount; i++ {
		for _, p := range r.Pages {
			if p.Index != i {
				continue
			}
			text := p.Text
			if strings.TrimSpace(text) == "" {
				text = "（未识别到文字）"
			}
			fmt.Fprintf(&out, "\n\n第%d页：\n%s", i+1, text)
			if p.Finish == "length" {
				out.WriteString("\n（该页识别结果被截断，不完整。）")
			}
		}
	}
	runes := []rune(out.String())
	if len(runes) > 12000 {
		return string(runes[:12000]) + "\n（结果较长，仅展示前12000字；请裁剪图片后分次识别。）"
	}
	return out.String()
}
func runOCR(ctx context.Context, cfg chat.AgentConfig, event *zero.Event, role goba.PermRole, client *http.Client, call func(context.Context, string, zero.Params) zero.APIResponse) (string, error) {
	if !linkAddressed(event) {
		return "", errors.New("群聊OCR需要@机器人。")
	}
	conversation := event.GroupID
	if conversation == 0 {
		conversation = -event.UserID
	}
	if !cfg.SearchAllowed(event.SelfID, conversation, role) {
		return "", errors.New("当前会话或角色未获准联网，请主人配置联网搜索权限。")
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	if client == nil {
		client = &http.Client{Timeout: 180 * time.Second}
	}
	generation := chat.AgentGeneration()
	source, e := ocrImageSource(ctx, event, call)
	if e != nil {
		return "", e
	}
	data, name, e := loadOCRImage(ctx, source, client)
	if e != nil {
		return "", e
	}
	if generation != chat.AgentGeneration() {
		return "", errors.New("配置已变化，本次OCR尚未提交，请重新发送命令。")
	}
	result, e := requestOCR(ctx, client, cfg.WebSearch.Key, data, name)
	if e != nil {
		return "", e
	}
	return ocrReply(result), nil
}
