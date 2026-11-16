package aichat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/FloatTech/zbputils/chat"
	goba "github.com/fumiama/go-onebot-agent"
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

func ttsPCM() []byte {
	b := make([]byte, 48000)
	for i := range b {
		b[i] = byte(i % 127)
	}
	return b
}
func TestTTSSynthesisAndCurrentConversationDelivery(t *testing.T) {
	for _, group := range []int64{10, 0} {
		calls, sends := 0, 0
		event := asrEvent()
		event.GroupID = group
		cfg := linkFixture()
		cfg.WebSearch.Scopes["456:-123"] = chat.WebSearchPolicy{Enabled: true, Roles: []string{"owner"}}
		client := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.String() != ttsEndpoint || r.Header.Get("Authorization") != "Bearer synthetic-search" {
				t.Fatal("endpoint/key")
			}
			var body map[string]any
			if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
				t.Fatal(e)
			}
			if len(body) != 4 || body["text"] != "合成测试" || body["voice_id"] != "tide" || body["format"] != "pcm" || body["filter_markdown"] != true {
				t.Fatal(body)
			}
			return &http.Response{StatusCode: 200, ContentLength: 48000, Header: http.Header{"Content-Type": []string{"audio/pcm"}}, Body: io.NopCloser(bytes.NewReader(ttsPCM()))}, nil
		})}
		audio, e := runTTS(context.Background(), cfg, event, goba.PermRoleOwner, "tide", "合成测试", client)
		if e != nil || calls != 1 || audio.Units != 8 || audio.Milliseconds != 1000 {
			t.Fatal(audio, e, calls)
		}
		e = sendTTSAudio(context.Background(), event, audio, func(_ context.Context, action string, p zero.Params) zero.APIResponse {
			sends++
			want := "send_private_msg"
			key := "user_id"
			target := int64(123)
			if group > 0 {
				want = "send_group_msg"
				key = "group_id"
				target = group
			}
			if action != want || p[key] != target || len(p) != 2 {
				t.Fatal(action, p)
			}
			m := p["message"].(message.Message)
			if len(m) != 1 || m[0].Type != "record" {
				t.Fatal(m)
			}
			b, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(m[0].Data["file"], "base64://"))
			if e != nil {
				t.Fatal(e)
			}
			if info, e := inspectASRWAV(b); e != nil || info.Milliseconds != 1000 {
				t.Fatal(info, e)
			}
			return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"message_id":-12}`)}
		})
		if e != nil || sends != 1 {
			t.Fatal(e, sends)
		}
	}
}

type brokenTTSBody struct{ sent bool }

func (b *brokenTTSBody) Read(p []byte) (int, error) {
	if b.sent {
		return 0, io.ErrUnexpectedEOF
	}
	b.sent = true
	return copy(p, []byte{1, 2, 3, 4}), nil
}
func (*brokenTTSBody) Close() error { return nil }
func TestTTSBadAudioNeverReturned(t *testing.T) {
	for _, tc := range []struct {
		status int
		media  string
		body   io.ReadCloser
		length int64
	}{
		{400, "application/json", io.NopCloser(strings.NewReader("error")), 5},
		{200, "application/json", io.NopCloser(strings.NewReader(`{"error":{}}`)), -1},
		{200, "audio/pcm", io.NopCloser(bytes.NewReader([]byte{1, 2, 3})), 3},
		{200, "audio/pcm", io.NopCloser(bytes.NewReader(nil)), 0},
		{200, "audio/pcm", &brokenTTSBody{}, -1},
		{200, "audio/pcm", io.NopCloser(bytes.NewReader([]byte{1, 2})), 4},
		{200, "audio/pcm", io.NopCloser(bytes.NewReader(nil)), ttsPCMLimit + 2},
		{302, "audio/pcm", io.NopCloser(bytes.NewReader(nil)), 0},
	} {
		calls := 0
		client := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: tc.status, ContentLength: tc.length, Header: http.Header{"Content-Type": []string{tc.media}, "Location": []string{"https://other.example"}}, Body: tc.body}, nil
		})}
		audio, e := requestTTS(context.Background(), client, "synthetic", "stella", "测试")
		if e == nil || len(audio.WAV) != 0 || calls != 1 {
			t.Fatal(e, calls)
		}
	}
}
func TestTTSAdmissionAndLegacyNames(t *testing.T) {
	for _, s := range []string{"发送AI语音 测试", "设置AI语音模型", "语音转文字", "ASR", "TTS设置"} {
		if _, _, ok := ttsCommand(s); ok {
			t.Fatal("legacy intercepted", s)
		}
	}
	for _, s := range []string{"TTS 测试", "tts 测试", "TTS男声　测试", "独立语音 测试"} {
		if _, text, ok := ttsCommand(s); !ok || text != "测试" {
			t.Fatal(s, text, ok)
		}
	}
	for _, s := range []string{"", strings.Repeat("中", 501), "a\x00b"} {
		if _, e := ttsUnits(s); e == nil {
			t.Fatal("invalid text")
		}
	}
	if n, e := ttsUnits(strings.Repeat("中", 500)); e != nil || n != 1000 {
		t.Fatal(n, e)
	}
	client := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("network forbidden")
		return nil, context.Canceled
	})}
	if _, e := runTTS(context.Background(), linkFixture(), asrEvent(), goba.PermRoleUser, "stella", "测试", client); e == nil {
		t.Fatal("role")
	}
	if _, e := requestTTS(context.Background(), client, "fake", "unknown", "测试"); e == nil {
		t.Fatal("voice")
	}
}
func TestTTSSendUnknownReceiptNoRetry(t *testing.T) {
	wav, ms, e := ttsPCMToWAV(ttsPCM())
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	e = sendTTSAudio(context.Background(), asrEvent(), ttsAudio{WAV: wav, Milliseconds: ms}, func(context.Context, string, zero.Params) zero.APIResponse {
		calls++
		return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"message_id":0}`)}
	})
	if e == nil || calls != 1 {
		t.Fatal(e, calls)
	}
}
func TestTTSNonIntegerReceiptUnconfirmed(t *testing.T) {
	wav, _, _ := ttsPCMToWAV(ttsPCM())
	calls := 0
	e := sendTTSAudio(context.Background(), asrEvent(), ttsAudio{WAV: wav}, func(context.Context, string, zero.Params) zero.APIResponse {
		calls++
		return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"message_id":12.5}`)}
	})
	if e == nil || calls != 1 {
		t.Fatal(e, calls)
	}
}
func TestTTSCancelledSendNeverCallsOneBot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wav, _, _ := ttsPCMToWAV(ttsPCM())
	e := sendTTSAudio(ctx, asrEvent(), ttsAudio{WAV: wav}, func(context.Context, string, zero.Params) zero.APIResponse {
		t.Fatal("cancelled send reached OneBot")
		return zero.APIResponse{}
	})
	if e == nil {
		t.Fatal("cancelled send accepted")
	}
}
func TestTTSDurationBoundary(t *testing.T) {
	wav, ms, e := ttsPCMToWAV(make([]byte, ttsPCMLimit))
	if e != nil || ms != 120000 {
		t.Fatal(ms, e)
	}
	if info, e := inspectASRWAV(wav); e != nil || info.Milliseconds != 120000 {
		t.Fatal(info, e)
	}
	if _, _, e := ttsPCMToWAV(make([]byte, ttsPCMLimit+2)); e == nil {
		t.Fatal("overlong audio accepted")
	}
}
func TestTTSTimeoutDoesNotRetry(t *testing.T) {
	calls := 0
	c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, context.DeadlineExceeded })}
	audio, e := requestTTS(context.Background(), c, "synthetic", "stella", "测试")
	if e == nil || len(audio.WAV) != 0 || calls != 1 {
		t.Fatal(e, calls)
	}
}
