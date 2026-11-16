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
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/FloatTech/zbputils/chat"
	goba "github.com/fumiama/go-onebot-agent"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const ttsEndpoint = "https://api.deepseek.com/v1/tools/tts"
const ttsPCMLimit = 24000 * 2 * 120

func ttsCommand(s string) (voice, text string, ok bool) {
	s = strings.TrimSpace(s)
	for _, c := range []struct{ prefix, voice string }{{"TTS男声", "tide"}, {"TTS女声", "stella"}, {"TTS", "stella"}, {"独立语音", "stella"}} {
		if len(s) < len(c.prefix) || !strings.EqualFold(s[:len(c.prefix)], c.prefix) {
			continue
		}
		tail := s[len(c.prefix):]
		r, _ := utf8.DecodeRuneInString(tail)
		if tail == "" || unicode.IsSpace(r) {
			return c.voice, strings.TrimSpace(tail), true
		}
	}
	return "", "", false
}
func ttsUnits(text string) (int, error) {
	if !utf8.ValidString(text) || strings.TrimSpace(text) == "" {
		return 0, errors.New("用法：TTS 要朗读的文字；也可用TTS男声或TTS女声。")
	}
	n := 0
	for _, r := range text {
		if r == 0 {
			return 0, errors.New("朗读文字包含无效字符。")
		}
		n++
		if unicode.Is(unicode.Han, r) {
			n++
		}
	}
	if n > 1000 {
		return 0, errors.New("本版TTS每次最多1000计费字符（汉字按2个），请拆分文字。")
	}
	return n, nil
}
func ttsPCMToWAV(pcm []byte) ([]byte, int64, error) {
	if len(pcm) == 0 || len(pcm)%2 != 0 || len(pcm) > ttsPCMLimit {
		return nil, 0, errors.New("合成音频为空、不完整或超过120秒，本次不发送。")
	}
	wav := make([]byte, 44+len(pcm))
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 24000)
	binary.LittleEndian.PutUint32(wav[28:], 48000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(pcm)))
	copy(wav[44:], pcm)
	return wav, (int64(len(pcm))*1000 + 47999) / 48000, nil
}

type ttsAudio struct {
	WAV          []byte
	Units        int
	Milliseconds int64
}

func requestTTS(ctx context.Context, client *http.Client, key chat.ModelKey, voice, text string) (ttsAudio, error) {
	result := ttsAudio{}
	units, e := ttsUnits(text)
	if e != nil {
		return result, e
	}
	if voice != "stella" && voice != "tide" {
		return result, errors.New("TTS音色无效。")
	}
	body, _ := json.Marshal(map[string]any{"text": text, "voice_id": voice, "format": "pcm", "filter_markdown": true})
	req, e := http.NewRequestWithContext(ctx, "POST", ttsEndpoint, bytes.NewReader(body))
	if e != nil {
		return result, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+string(key))
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	r, e := c.Do(req)
	if e != nil {
		return result, errors.New("TTS连接失败或超时，未自动重试。")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return result, fmt.Errorf("TTS服务返回HTTP %d，请检查文本、审核、余额或额度；未自动重试。", r.StatusCode)
	}
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || (media != "audio/pcm" && media != "audio/x-pcm" && media != "application/octet-stream") {
		return result, errors.New("TTS未返回预期的PCM音频，本次不发送。")
	}
	if r.ContentLength > ttsPCMLimit {
		return result, errors.New("TTS音频超过120秒，本次不发送。")
	}
	pcm, e := io.ReadAll(io.LimitReader(r.Body, ttsPCMLimit+1))
	if e != nil {
		return result, errors.New("TTS音频传输中断，未发送且未自动重试。")
	}
	if r.ContentLength >= 0 && int64(len(pcm)) != r.ContentLength {
		return result, errors.New("TTS音频长度不完整，本次不发送。")
	}
	wav, ms, e := ttsPCMToWAV(pcm)
	if e != nil {
		return result, e
	}
	return ttsAudio{wav, units, ms}, nil
}
func runTTS(ctx context.Context, cfg chat.AgentConfig, event *zero.Event, role goba.PermRole, voice, text string, client *http.Client) (ttsAudio, error) {
	conv := event.GroupID
	if conv == 0 {
		conv = -event.UserID
	}
	if !linkAddressed(event) || !cfg.SearchAllowed(event.SelfID, conv, role) {
		return ttsAudio{}, errors.New("独立TTS需要当前会话联网权限，群聊还需@机器人。")
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	generation := chat.AgentGeneration()
	audio, e := requestTTS(ctx, client, cfg.WebSearch.Key, voice, text)
	if e != nil {
		return ttsAudio{}, e
	}
	if generation != chat.AgentGeneration() {
		return ttsAudio{}, errors.New("配置已变化，合成结果未发送。")
	}
	return audio, nil
}
func sendTTSAudio(ctx context.Context, event *zero.Event, audio ttsAudio, call func(context.Context, string, zero.Params) zero.APIResponse) error {
	if err := ctx.Err(); err != nil {
		return errors.New("TTS发送已取消，未发送语音。")
	}
	if _, e := inspectASRWAV(audio.WAV); e != nil {
		return errors.New("合成音频校验失败，未发送。")
	}
	action := "send_private_msg"
	p := zero.Params{"user_id": event.UserID}
	if event.GroupID > 0 {
		action = "send_group_msg"
		p = zero.Params{"group_id": event.GroupID}
	} else if event.GroupID < 0 || event.UserID <= 0 {
		return errors.New("语音发送目标无效。")
	}
	p["message"] = message.Message{message.Record("base64://" + base64.StdEncoding.EncodeToString(audio.WAV))}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r := call(ctx, action, p)
	id, idErr := strconv.ParseInt(r.Data.Get("message_id").String(), 10, 64)
	if r.Status != "ok" || r.RetCode != 0 || idErr != nil || id == 0 {
		return errors.New("TTS语音发送失败或回执未确认，未自动重试；请先检查会话中是否已收到。")
	}
	return nil
}
