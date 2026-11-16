package aichat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	goba "github.com/fumiama/go-onebot-agent"
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func asrWAV(seconds int) []byte {
	n := 16000 * 2 * seconds
	b := make([]byte, 44+n)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(n))
	return b
}
func asrEvent() *zero.Event {
	return &zero.Event{SelfID: 456, GroupID: 10, UserID: 123, NativeMessage: []byte(`[{"type":"at","data":{"qq":"456"}},{"type":"reply","data":{"id":"-12"}},{"type":"text","data":{"text":"ASR"}}]`)}
}
func asrOneBot(t *testing.T, data []byte, actions *[]string) func(context.Context, string, zero.Params) zero.APIResponse {
	return func(_ context.Context, action string, p zero.Params) zero.APIResponse {
		*actions = append(*actions, action)
		switch action {
		case "get_msg":
			if p["message_id"] != int64(-12) {
				t.Fatal(p)
			}
			return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"message_id":-12,"group_id":10,"message":[{"type":"record","data":{"file":"synthetic.amr"}}]}`)}
		case "get_record":
			if p["file"] != "synthetic.amr" || p["out_format"] != "wav" {
				t.Fatal(p)
			}
			b, _ := json.Marshal(map[string]string{"base64": base64.StdEncoding.EncodeToString(data), "file": "C:/never-read.wav"})
			return zero.APIResponse{Status: "ok", Data: gjson.ParseBytes(b)}
		default:
			t.Fatal("unexpected action", action)
			return zero.APIResponse{}
		}
	}
}
func TestASRPipelineNoLegacyOrModel(t *testing.T) {
	data := asrWAV(1)
	actions := []string{}
	calls := 0
	c := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != asrEndpoint || r.Header.Get("Authorization") != "Bearer synthetic-search" {
			t.Fatal("wrong endpoint/key")
		}
		if e := r.ParseMultipartForm(1 << 20); e != nil {
			t.Fatal(e)
		}
		defer r.MultipartForm.RemoveAll()
		if len(r.MultipartForm.Value) != 0 || len(r.MultipartForm.File) != 1 {
			t.Fatal("unexpected fields")
		}
		f, h, e := r.FormFile("file")
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		if !bytes.Equal(b, data) || h.Filename != "speech.wav" {
			t.Fatal("bad upload")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"request_id":"asr-test","text":"合成转写文字","duration_ms":1000}`))}, nil
	})}
	out, e := runASR(context.Background(), linkFixture(), asrEvent(), goba.PermRoleOwner, c, asrOneBot(t, data, &actions))
	if e != nil || calls != 1 || strings.Join(actions, ",") != "get_msg,get_record" || !strings.Contains(out, "合成转写文字") {
		t.Fatal(out, e, calls, actions)
	}
}
func TestASRWAVValidation(t *testing.T) {
	for _, seconds := range []int{1, 120} {
		if info, e := inspectASRWAV(asrWAV(seconds)); e != nil || info.Milliseconds != int64(seconds*1000) {
			t.Fatal(info, e)
		}
	}
	cases := [][]byte{nil, []byte("not wave"), asrWAV(121), asrWAV(1)[:44]}
	badBits := asrWAV(1)
	binary.LittleEndian.PutUint16(badBits[34:], 24)
	cases = append(cases, badBits)
	badRate := asrWAV(1)
	binary.LittleEndian.PutUint32(badRate[24:], 12345)
	cases = append(cases, badRate)
	badSize := asrWAV(1)
	binary.LittleEndian.PutUint32(badSize[40:], 0xffffffff)
	cases = append(cases, badSize)
	for _, b := range cases {
		if _, e := inspectASRWAV(b); e == nil {
			t.Fatal("invalid WAV accepted")
		}
	}
}
func TestASRAdmissionAndFormatBeforeUpload(t *testing.T) {
	c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("network forbidden")
		return nil, context.Canceled
	})}
	if _, e := runASR(context.Background(), linkFixture(), asrEvent(), goba.PermRoleUser, c, noOneBot(t)); e == nil {
		t.Fatal("role")
	}
	actions := []string{}
	if _, e := runASR(context.Background(), linkFixture(), asrEvent(), goba.PermRoleOwner, c, asrOneBot(t, asrWAV(121), &actions)); e == nil {
		t.Fatal("long clip")
	}
	for _, source := range []string{"C:/private.wav", "https://example.com/speech.wav", "base64://abc"} {
		raw, _ := json.Marshal([]any{map[string]any{"type": "record", "data": map[string]string{"file": source}}})
		event := asrEvent()
		event.NativeMessage = raw
		if _, e := asrRecordID(context.Background(), event, noOneBot(t)); e == nil {
			t.Fatal(source)
		}
	}
	event := asrEvent()
	call := func(context.Context, string, zero.Params) zero.APIResponse {
		return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"message_id":-12,"group_id":20,"message":[{"type":"record","data":{"file":"x.amr"}}]}`)}
	}
	if _, e := asrRecordID(context.Background(), event, call); e == nil {
		t.Fatal("cross group")
	}
}
func TestASRErrorsNoRetry(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{429, ""}, {200, `{"error":{"message":"private detail"}}`}, {200, `{}`}, {200, `{"request_id":"x","text":"x","duration_ms":120001}`}, {302, ""}} {
		calls := 0
		c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) {
			calls++
			h := make(http.Header)
			h.Set("Location", "https://evil.example")
			return &http.Response{StatusCode: tc.status, Header: h, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})}
		if _, e := requestASR(context.Background(), c, "synthetic", asrWAV(1)); e == nil || calls != 1 {
			t.Fatal(e, calls)
		}
	}
}
func TestASRPreservesLegacyCommands(t *testing.T) {
	for _, s := range []string{"转文字", "转写", "语音转文字", "帮我转文字"} {
		if asrCommand(s) || !transcriptOnly(s) {
			t.Fatal("legacy command changed", s)
		}
	}
	for _, s := range []string{"ASR", "asr", "独立语音转文字"} {
		if !asrCommand(s) || transcriptOnly(s) {
			t.Fatal(s)
		}
	}
}
func TestASRPrivateQuotedScope(t *testing.T) {
	event := asrEvent()
	event.GroupID = 0
	for _, uid := range []int64{123, 999} {
		call := func(context.Context, string, zero.Params) zero.APIResponse {
			b, _ := json.Marshal(map[string]any{"message_id": -12, "message_type": "private", "sender": map[string]int64{"user_id": uid}, "message": []any{map[string]any{"type": "record", "data": map[string]string{"file": "test.amr"}}}})
			return zero.APIResponse{Status: "ok", Data: gjson.ParseBytes(b)}
		}
		id, e := asrRecordID(context.Background(), event, call)
		if (e == nil) != (uid == 123) || (e == nil && id != "test.amr") {
			t.Fatal(uid, id, e)
		}
	}
}
func TestLegacyTranscriptionStillUsesFetchPTT(t *testing.T) {
	actions := []string{}
	call := func(_ context.Context, action string, _ zero.Params) zero.APIResponse {
		actions = append(actions, action)
		if action == "get_msg" {
			return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"message_id":12,"group_id":10,"message":[{"type":"record","data":{"file":"old.amr"}}]}`)}
		}
		if action != "fetch_ptt_text" {
			t.Fatal(action)
		}
		return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"text":"原转写结果"}`)}
	}
	text, voice, e := readQuotedVoice(10, 12, call)
	if e != nil || !voice || text != "原转写结果" || strings.Join(actions, ",") != "get_msg,fetch_ptt_text" {
		t.Fatal(text, voice, e, actions)
	}
}
func TestASRMissingTextIsNotEmptyTranscript(t *testing.T) {
	for _, body := range []string{`{"request_id":"x","duration_ms":1000}`, `{"request_id":"x","duration_ms":1000,"text":null}`, `{"request_id":"x","duration_ms":1000,"text":""}`} {
		calls := 0
		c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		result, e := requestASR(context.Background(), c, "synthetic", asrWAV(1))
		wantOK := strings.Contains(body, `"text":""`)
		if (e == nil) != wantOK || calls != 1 {
			t.Fatal(body, result, e, calls)
		}
	}
}
func TestASRStereoAndAncillaryChunk(t *testing.T) {
	stereo := asrWAV(1)
	binary.LittleEndian.PutUint16(stereo[22:], 2)
	binary.LittleEndian.PutUint16(stereo[32:], 4)
	binary.LittleEndian.PutUint32(stereo[28:], 64000)
	info, e := inspectASRWAV(stereo)
	if e != nil || info.Channels != 2 || info.Milliseconds != 500 {
		t.Fatal(info, e)
	}
	b := asrWAV(1)
	chunk := []byte{'J', 'U', 'N', 'K', 1, 0, 0, 0, 42, 0}
	b = append(b, chunk...)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	if _, e = inspectASRWAV(b); e != nil {
		t.Fatal("valid padded ancillary chunk", e)
	}
	b = b[:len(b)-1]
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	if _, e = inspectASRWAV(b); e == nil {
		t.Fatal("missing pad accepted")
	}
}
func TestASRTransportTimeoutNoRetry(t *testing.T) {
	calls := 0
	c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, context.DeadlineExceeded })}
	if _, e := requestASR(context.Background(), c, "synthetic", asrWAV(1)); e == nil || calls != 1 {
		t.Fatal(e, calls)
	}
}
