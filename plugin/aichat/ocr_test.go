package aichat

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	goba "github.com/fumiama/go-onebot-agent"
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func ocrPNG() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 3)))
	return b.Bytes()
}
func ocrEvent(source string) *zero.Event {
	raw, _ := json.Marshal([]any{map[string]any{"type": "at", "data": map[string]string{"qq": "456"}}, map[string]any{"type": "text", "data": map[string]string{"text": "OCR"}}, map[string]any{"type": "image", "data": map[string]string{"file": source}}})
	return &zero.Event{SelfID: 456, GroupID: 10, UserID: 123, NativeMessage: raw}
}

const ocrOK = `{"id":"ocr-test","object":"ocr.result","page_count":1,"pages":[{"index":0,"text":"合成测试文字 [CQ:at,qq=all]","finish_reason":"stop"}],"usage":{"input_tokens":9,"output_tokens":3}}`

func noOneBot(t *testing.T) func(context.Context, string, zero.Params) zero.APIResponse {
	return func(context.Context, string, zero.Params) zero.APIResponse {
		t.Fatal("unexpected OneBot call")
		return zero.APIResponse{}
	}
}
func TestOCRSingleImageMultipartNoChat(t *testing.T) {
	calls := 0
	data := ocrPNG()
	c := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != ocrEndpoint || r.Header.Get("Authorization") != "Bearer synthetic-search" {
			t.Fatal("wrong endpoint or key")
		}
		if e := r.ParseMultipartForm(1 << 20); e != nil {
			t.Fatal(e)
		}
		defer r.MultipartForm.RemoveAll()
		if len(r.MultipartForm.Value) != 1 || r.FormValue("purpose") != "text_recognition" || len(r.MultipartForm.File) != 1 {
			t.Fatal("unexpected form fields")
		}
		f, h, e := r.FormFile("file")
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		b, _ := io.ReadAll(f)
		if !bytes.Equal(b, data) || h.Filename != "image.png" {
			t.Fatal("image corrupted")
		}
		if _, ok := r.Context().Deadline(); !ok {
			t.Fatal("deadline missing")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("\n\n" + ocrOK))}, nil
	})}
	out, e := runOCR(context.Background(), linkFixture(), ocrEvent("base64://"+base64.StdEncoding.EncodeToString(data)), goba.PermRoleOwner, c, noOneBot(t))
	if e != nil || calls != 1 || !strings.Contains(out, "合成测试文字") {
		t.Fatal(out, e, calls)
	}
}
func TestOCRPermissionAndInputNoRequest(t *testing.T) {
	c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("network forbidden")
		return nil, context.Canceled
	})}
	event := ocrEvent("base64://" + base64.StdEncoding.EncodeToString(ocrPNG()))
	if _, e := runOCR(context.Background(), linkFixture(), event, goba.PermRoleUser, c, noOneBot(t)); e == nil {
		t.Fatal("role")
	}
	event.NativeMessage = []byte(`[{"type":"text","data":{"text":"OCR"}}]`)
	if _, e := runOCR(context.Background(), linkFixture(), event, goba.PermRoleOwner, c, noOneBot(t)); e == nil {
		t.Fatal("mention")
	}
	for _, s := range []string{"file:///private.png", "http://localhost/x", "https://evil.example/x", "https://qpic.cn.evil.example/x", "https://user:pass@gchat.qpic.cn/x", "base64://!", "base64://" + base64.StdEncoding.EncodeToString([]byte("not image"))} {
		if _, _, e := loadOCRImage(context.Background(), s, c); e == nil {
			t.Fatal(s)
		}
	}
}
func TestOCRAttachmentDownloadDoesNotReceiveKey(t *testing.T) {
	calls := 0
	c := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "" || r.Method != "GET" {
			t.Fatal("credential leaked")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(ocrPNG()))}, nil
	})}
	if _, _, e := loadOCRImage(context.Background(), "https://gchat.qpic.cn/image", c); e != nil || calls != 1 {
		t.Fatal(e, calls)
	}
}
func TestOCRErrorsAndPartialPages(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{402, ""}, {429, ""}, {200, `{"error":{"message":"private provider detail"}}`}, {200, `{}`}, {200, `{"id":"x","object":"ocr.result","page_count":2,"pages":[{"index":0,"text":"x","finish_reason":"stop"}]}`},
		{200, strings.Replace(ocrOK, `"index":0`, `"index":2`, 1)},
	} {
		calls := 0
		c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})}
		if _, e := requestOCR(context.Background(), c, "synthetic", ocrPNG(), "image.png"); e == nil || calls != 1 {
			t.Fatal(e, calls)
		}
	}
	var r ocrResult
	_ = json.Unmarshal([]byte(strings.Replace(ocrOK, `"stop"`, `"length"`, 1)), &r)
	if !strings.Contains(ocrReply(r), "不完整") {
		t.Fatal("truncation not marked")
	}
	r.Pages[0].Text = ""
	if !strings.Contains(ocrReply(r), "未识别到文字") {
		t.Fatal("empty page")
	}
}
func TestOCRQuotedScopeAndImageCount(t *testing.T) {
	event := &zero.Event{SelfID: 456, GroupID: 10, UserID: 123, NativeMessage: []byte(`[{"type":"reply","data":{"id":"12"}}]`)}
	call := func(context.Context, string, zero.Params) zero.APIResponse {
		return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"message_id":12,"group_id":20,"message":[{"type":"image","data":{"file":"x"}}]}`)}
	}
	if _, e := ocrImageSource(context.Background(), event, call); e == nil {
		t.Fatal("cross group")
	}
	event.GroupID = 0
	call = func(context.Context, string, zero.Params) zero.APIResponse {
		return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"message_id":12,"message_type":"private","sender":{"user_id":999},"message":[{"type":"image","data":{"file":"x"}}]}`)}
	}
	if _, e := ocrImageSource(context.Background(), event, call); e == nil {
		t.Fatal("cross private")
	}
	event.NativeMessage = []byte(`[{"type":"image","data":{"file":"x"}},{"type":"image","data":{"file":"y"}}]`)
	if _, e := ocrImageSource(context.Background(), event, noOneBot(t)); e == nil {
		t.Fatal("multiple images")
	}
}
func TestOCRSizeRedirectAndTimeout(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		size   int64
		fail   bool
	}{
		{"large", 200, ocrImageLimit + 1, false}, {"redirect", 302, 0, false}, {"timeout", 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if tc.fail {
					return nil, context.DeadlineExceeded
				}
				h := make(http.Header)
				h.Set("Location", "https://other.example/image")
				return &http.Response{StatusCode: tc.status, Header: h, ContentLength: tc.size, Body: io.NopCloser(strings.NewReader(""))}, nil
			})}
			if _, _, e := loadOCRImage(context.Background(), "https://gchat.qpic.cn/x", c); e == nil || calls != 1 {
				t.Fatal(e, calls)
			}
		})
	}
	calls := 0
	c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, context.DeadlineExceeded })}
	if _, e := requestOCR(context.Background(), c, "fake", ocrPNG(), "image.png"); e == nil || calls != 1 {
		t.Fatal(e, calls)
	}
}
func TestOCRQuotedImageSuccessAndResolve(t *testing.T) {
	event := &zero.Event{SelfID: 456, GroupID: 10, UserID: 123, NativeMessage: []byte(`[{"type":"reply","data":{"id":"12"}}]`)}
	calls := 0
	call := func(_ context.Context, action string, _ zero.Params) zero.APIResponse {
		calls++
		if action == "get_msg" {
			return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"message_id":12,"group_id":10,"message":[{"type":"image","data":{"file":"cached-image"}}]}`)}
		}
		if action != "get_image" {
			t.Fatal(action)
		}
		return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"url":"https://gchat.qpic.cn/x","file":"C:/private.png"}`)}
	}
	s, e := ocrImageSource(context.Background(), event, call)
	if e != nil || s != "https://gchat.qpic.cn/x" || calls != 2 {
		t.Fatal(s, e, calls)
	}
}
func TestOCRRejectsTruncatedImageBeforeUpload(t *testing.T) {
	data := ocrPNG()
	// IHDR survives, so DecodeConfig alone accepts this damaged PNG.
	cut := data[:33]
	if _, _, e := image.DecodeConfig(bytes.NewReader(cut)); e != nil {
		t.Fatal("fixture header invalid", e)
	}
	calls := 0
	c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, context.Canceled })}
	_, e := runOCR(context.Background(), linkFixture(), ocrEvent("base64://"+base64.StdEncoding.EncodeToString(cut)), goba.PermRoleOwner, c, noOneBot(t))
	if e == nil || calls != 0 {
		t.Fatal("damaged image uploaded", e, calls)
	}
}
func TestOCRNegativeOneBotMessageID(t *testing.T) {
	event := &zero.Event{SelfID: 456, GroupID: 10, UserID: 123, NativeMessage: []byte(`[{"type":"reply","data":{"id":"-12"}}]`)}
	call := func(_ context.Context, action string, p zero.Params) zero.APIResponse {
		if action != "get_msg" || p["message_id"] != int64(-12) {
			t.Fatal(action, p)
		}
		return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"message_id":-12,"group_id":10,"message":[{"type":"image","data":{"url":"https://gchat.qpic.cn/x"}}]}`)}
	}
	s, e := ocrImageSource(context.Background(), event, call)
	if e != nil || s != "https://gchat.qpic.cn/x" {
		t.Fatal(s, e)
	}
}
