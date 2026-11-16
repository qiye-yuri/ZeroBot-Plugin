package aichat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FloatTech/zbputils/chat"
	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const asrEndpoint = "https://api.deepseek.com/v1/tools/asr"
const asrByteLimit = 8 << 20

func asrCommand(s string) bool {
	s = strings.TrimSpace(s)
	return strings.EqualFold(s, "ASR") || s == "独立语音转文字"
}
func asrRecordID(ctx context.Context, event *zero.Event, call func(context.Context, string, zero.Params) zero.APIResponse) (string, error) {
	segs := message.ParseMessage(event.NativeMessage)
	ids := []string{}
	reply := ""
	collect := func(m message.Message) {
		for _, s := range m {
			if s.Type == "record" {
				ids = append(ids, s.Data["file"])
			}
		}
	}
	collect(segs)
	for _, s := range segs {
		if s.Type == "reply" {
			if reply != "" {
				return "", errors.New("请一次只引用一条语音消息。")
			}
			reply = s.Data["id"]
		}
	}
	if len(ids) == 0 && reply != "" {
		id, e := strconv.ParseInt(reply, 10, 64)
		if e != nil || id == 0 {
			return "", errors.New("语音引用编号无效。")
		}
		r := call(ctx, "get_msg", zero.Params{"message_id": id})
		if r.Status != "ok" || r.RetCode != 0 || r.Data.Get("message_id").Int() != id {
			return "", errors.New("无法读取引用语音，请重新发送。")
		}
		if event.GroupID != 0 {
			if r.Data.Get("group_id").Int() != event.GroupID {
				return "", errors.New("只能识别本群引用语音。")
			}
		} else {
			if r.Data.Get("group_id").Int() != 0 || r.Data.Get("message_type").String() != "private" || r.Data.Get("sender.user_id").Int() != event.UserID {
				return "", errors.New("只能识别当前私聊用户发送的语音。")
			}
		}
		collect(message.ParseMessage([]byte(r.Data.Get("message").Raw)))
	}
	if len(ids) != 1 || ids[0] == "" || len(ids[0]) > 512 || strings.ContainsAny(ids[0], "/\\:\x00\r\n") {
		return "", errors.New("请引用一条QQ语音并发送ASR；不接受路径、网址或多条语音。")
	}
	return ids[0], nil
}

type asrAudioInfo struct {
	Milliseconds int64
	Channels     int
}

func inspectASRWAV(b []byte) (asrAudioInfo, error) {
	bad := errors.New("语音转换结果无效，需要完整的16位PCM WAV。")
	info := asrAudioInfo{}
	if len(b) < 44 || len(b) > asrByteLimit || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" || uint64(binary.LittleEndian.Uint32(b[4:8]))+8 != uint64(len(b)) {
		return info, bad
	}
	var rate, byteRate uint32
	var align uint16
	dataSize := 0
	hasFmt, hasData := false, false
	for pos := 12; pos < len(b); {
		if len(b)-pos < 8 {
			return info, bad
		}
		n := int64(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
		start := pos + 8
		end := int64(start) + n
		if end > int64(len(b)) {
			return info, bad
		}
		switch string(b[pos : pos+4]) {
		case "fmt ":
			if hasFmt || n < 16 {
				return info, bad
			}
			hasFmt = true
			p := b[start:int(end)]
			if binary.LittleEndian.Uint16(p[:2]) != 1 || binary.LittleEndian.Uint16(p[14:16]) != 16 {
				return info, bad
			}
			info.Channels = int(binary.LittleEndian.Uint16(p[2:4]))
			rate = binary.LittleEndian.Uint32(p[4:8])
			byteRate = binary.LittleEndian.Uint32(p[8:12])
			align = binary.LittleEndian.Uint16(p[12:14])
		case "data":
			if hasData || n == 0 {
				return info, bad
			}
			hasData = true
			dataSize = int(n)
		}
		pos = int(end + n%2)
		if pos > len(b) {
			return info, bad
		}
	}
	validRate := false
	for _, r := range []uint32{8000, 16000, 22050, 24000, 32000, 44100, 48000} {
		validRate = validRate || rate == r
	}
	if !hasFmt || !hasData || !validRate || (info.Channels != 1 && info.Channels != 2) || align != uint16(info.Channels*2) || byteRate != rate*uint32(align) || dataSize%int(align) != 0 {
		return info, bad
	}
	if int64(dataSize) > int64(byteRate)*120 {
		return info, errors.New("独立ASR最多识别120秒，请先分段。")
	}
	info.Milliseconds = (int64(dataSize)*1000 + int64(byteRate) - 1) / int64(byteRate)
	return info, nil
}
func loadASRAudio(ctx context.Context, id string, call func(context.Context, string, zero.Params) zero.APIResponse) ([]byte, asrAudioInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r := call(ctx, "get_record", zero.Params{"file": id, "out_format": "wav"})
	if r.Status != "ok" || r.RetCode != 0 {
		return nil, asrAudioInfo{}, errors.New("QQ语音转换失败，请检查连接端的WAV转换能力；原转写命令仍可使用。")
	}
	encoded := r.Data.Get("base64").String()
	if len(encoded) == 0 || len(encoded) > base64.StdEncoding.EncodedLen(asrByteLimit) {
		return nil, asrAudioInfo{}, errors.New("转换后的语音为空或超过8MiB。")
	}
	data, e := base64.StdEncoding.DecodeString(encoded)
	if e != nil {
		return nil, asrAudioInfo{}, errors.New("语音转换数据无效。")
	}
	info, e := inspectASRWAV(data)
	return data, info, e
}

type asrResult struct {
	RequestID  string          `json:"request_id"`
	Text       *string         `json:"text"`
	DurationMS *int64          `json:"duration_ms"`
	Error      json.RawMessage `json:"error"`
}

func requestASR(ctx context.Context, c *http.Client, key chat.ModelKey, data []byte) (asrResult, error) {
	var result asrResult
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	f, e := w.CreateFormFile("file", "speech.wav")
	if e != nil {
		return result, e
	}
	if _, e = f.Write(data); e != nil {
		return result, e
	}
	if e = w.Close(); e != nil {
		return result, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", asrEndpoint, &body)
	if e != nil {
		return result, e
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+string(key))
	client := *c
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	r, e := client.Do(req)
	if e != nil {
		return result, errors.New("ASR连接失败或超时，未自动重试。")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return result, fmt.Errorf("ASR服务返回HTTP %d，请检查音频、余额或额度；未自动重试。", r.StatusCode)
	}
	raw, e := io.ReadAll(io.LimitReader(r.Body, (256<<10)+1))
	if e != nil || len(raw) > 256<<10 {
		return result, errors.New("ASR响应不完整或过大。")
	}
	if json.Unmarshal(raw, &result) != nil || (len(result.Error) > 0 && string(result.Error) != "null") || result.RequestID == "" || result.DurationMS == nil || *result.DurationMS <= 0 || *result.DurationMS > 120000 || result.Text == nil || !utf8.ValidString(*result.Text) {
		return result, errors.New("ASR返回错误或无效识别结果，未自动重试。")
	}
	if len([]rune(*result.Text)) > 12000 {
		return result, errors.New("ASR识别文字超过显示上限，请分段识别。")
	}
	return result, nil
}
func runASR(ctx context.Context, cfg chat.AgentConfig, event *zero.Event, role goba.PermRole, client *http.Client, call func(context.Context, string, zero.Params) zero.APIResponse) (string, error) {
	conversation := event.GroupID
	if conversation == 0 {
		conversation = -event.UserID
	}
	if !linkAddressed(event) || !cfg.SearchAllowed(event.SelfID, conversation, role) {
		return "", errors.New("独立ASR需要当前会话联网权限，群聊还需@机器人。")
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	generation := chat.AgentGeneration()
	id, e := asrRecordID(ctx, event, call)
	if e != nil {
		return "", e
	}
	data, info, e := loadASRAudio(ctx, id, call)
	if e != nil {
		return "", e
	}
	if generation != chat.AgentGeneration() {
		return "", errors.New("配置已变化，ASR尚未提交，请重新发送命令。")
	}
	result, e := requestASR(ctx, client, cfg.WebSearch.Key, data)
	if e != nil {
		return "", e
	}
	if generation != chat.AgentGeneration() {
		return "", errors.New("配置已变化，本次结果已丢弃。")
	}
	text := strings.TrimSpace(*result.Text)
	if text == "" {
		text = "（未识别到可转写的语音内容）"
	}
	note := ""
	if info.Channels == 2 {
		note = "\n（双声道音频仅识别左声道。）"
	}
	return fmt.Sprintf("ASR转写（服务端音频时长%.3f秒）：\n%s%s", float64(*result.DurationMS)/1000, text, note), nil
}
