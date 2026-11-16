package aichat

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	goba "github.com/fumiama/go-onebot-agent"
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
)

func documentFixture(ext string) []byte {
	switch ext {
	case ".pdf":
		return []byte("%PDF-1.4\n% synthetic fixture\n%%EOF")
	case ".doc", ".ppt":
		return append([]byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}, []byte("synthetic compound fixture")...)
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, _ := z.Create("[Content_Types].xml")
	f.Write([]byte("<Types/>"))
	name := "word/document.xml"
	if ext == ".pptx" {
		name = "ppt/presentation.xml"
	}
	f, _ = z.Create(name)
	f.Write([]byte("<document>合成文件内容</document>"))
	z.Close()
	return b.Bytes()
}
func TestDocumentQAFormatsPipeline(t *testing.T) {
	for _, ext := range []string{".pdf", ".doc", ".docx", ".ppt", ".pptx"} {
		t.Run(ext, func(t *testing.T) {
			scope := documentKey{456, 10, 123, "synthetic"}
			rememberDocument(scope, documentInput{ID: "fixture-id", Name: "test" + ext, Size: 100, BusID: 102}, time.Now())
			cfg := linkFixture()
			cfg.API = "https://model.example/chat/completions"
			calls := []string{}
			data := documentFixture(ext)
			client := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
				calls = append(calls, r.URL.String())
				body := ""
				switch r.URL.Host {
				case "files.qq.com":
					if r.Header.Get("Authorization") != "" {
						t.Fatal("key leaked to attachment")
					}
					body = string(data)
				case "api.deepseek.com":
					if r.URL.String() != ocrEndpoint || r.Header.Get("Authorization") != "Bearer synthetic-search" {
						t.Fatal("bad OCR")
					}
					if e := r.ParseMultipartForm(1 << 20); e != nil {
						t.Fatal(e)
					}
					defer r.MultipartForm.RemoveAll()
					f, h, e := r.FormFile("file")
					if e != nil {
						t.Fatal(e)
					}
					b, _ := io.ReadAll(f)
					f.Close()
					if !bytes.Equal(b, data) || h.Filename != "document"+ext {
						t.Fatal("upload mismatch")
					}
					body = strings.Replace(ocrOK, "合成测试文字 [CQ:at,qq=all]", "项目计划在周六完成。", 1)
				case "model.example":
					b, _ := io.ReadAll(r.Body)
					if !strings.Contains(string(b), "周六") || !strings.Contains(string(b), "第1页") || strings.Contains(string(b), "fixture-id") {
						t.Fatal("document context boundary")
					}
					body = `{"choices":[{"message":{"content":"周六完成。[第1页]"}}]}`
				default:
					t.Fatal("unexpected network", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			onebot := func(_ context.Context, action string, p zero.Params) zero.APIResponse {
				if action != "get_group_file_url" || p["group_id"] != int64(10) || p["file_id"] != "fixture-id" {
					t.Fatal(action, p)
				}
				return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"url":"https://files.qq.com/test"}`)}
			}
			out, e := runDocumentQA(context.Background(), cfg, ocrEvent("unused"), scope, goba.PermRoleOwner, false, "哪天完成？", client, onebot)
			if e != nil || !strings.Contains(out, "周六") || len(calls) != 3 {
				t.Fatal(out, e, calls)
			}
		})
	}
}
func TestDocumentScopeExpiryAndInvalidTypes(t *testing.T) {
	now := time.Now()
	key := documentKey{456, 10, 123, "batch-a"}
	rememberDocument(key, documentInput{ID: "one", Name: "x.pdf"}, now)
	for _, k := range []documentKey{{457, 10, 123, "batch-a"}, {456, 11, 123, "batch-a"}, {456, 10, 124, "batch-a"}, {456, 10, 123, "batch-b"}} {
		if _, ok := currentDocument(k, now); ok {
			t.Fatal("scope leak", k)
		}
	}
	if _, ok := currentDocument(key, now.Add(15*time.Minute)); ok {
		t.Fatal("expired")
	}
	for _, u := range []string{"file:///x", "http://127.0.0.1/x", "https://files.qq.com.evil.example/x", "https://user:secret@files.qq.com/x"} {
		if documentURLAllowed(u) {
			t.Fatal(u)
		}
	}
	for _, ext := range []string{".exe", ".xlsx", ".pdf", ".docx"} {
		if validateDocument([]byte("invalid"), "x"+ext) == nil {
			t.Fatal(ext)
		}
	}
	if validateDocument(documentFixture(".docx"), "x.pptx") == nil {
		t.Fatal("wrong office kind")
	}
}
func TestDocumentFailureStopsBeforeModel(t *testing.T) {
	scope := documentKey{456, 10, 123, "failure"}
	rememberDocument(scope, documentInput{ID: "x", Name: "x.pdf"}, time.Now())
	for _, failure := range []string{`{"error":{"message":"password protected"}}`, strings.Replace(ocrOK, `"stop"`, `"length"`, 1), strings.Replace(ocrOK, "合成测试文字 [CQ:at,qq=all]", "", 1)} {
		calls := 0
		c := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			body := failure
			if r.URL.Host == "files.qq.com" {
				body = string(documentFixture(".pdf"))
			} else if r.URL.String() != ocrEndpoint {
				t.Fatal("model must not run")
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})}
		onebot := func(context.Context, string, zero.Params) zero.APIResponse {
			return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"url":"https://files.qq.com/x"}`)}
		}
		if _, e := runDocumentQA(context.Background(), linkFixture(), ocrEvent("unused"), scope, goba.PermRoleOwner, false, "总结", c, onebot); e == nil || calls != 2 {
			t.Fatal(e, calls)
		}
	}
}
func TestObserveDocumentMetadata(t *testing.T) {
	raw, _ := json.Marshal([]any{map[string]any{"type": "file", "data": map[string]string{"file_id": "private-fixture", "name": "test.docx", "file_size": "128"}}})
	ctx := &zero.Ctx{Event: &zero.Event{SelfID: 456, UserID: 123, NativeMessage: raw}, State: zero.State{}}
	observeDocument(ctx)
	d, ok := currentDocument(documentScope(ctx), time.Now())
	if !ok || d.ID != "private-fixture" || d.Name != "test.docx" {
		t.Fatal(d, ok)
	}
}
func TestDocumentNativeGroupAndPrivateMetadata(t *testing.T) {
	for _, raw := range []string{
		`{"post_type":"notice","notice_type":"group_upload","self_id":456,"group_id":10,"user_id":123,"file":{"id":"group-file","name":"test.pdf","size":512,"busid":102}}`,
		`{"post_type":"message","message_type":"private","self_id":456,"user_id":123,"message":[{"type":"file","data":{"file":"test.pptx","file_id":"private-file","file_size":"512"}}]}`,
	} {
		var event zero.Event
		if e := json.Unmarshal([]byte(raw), &event); e != nil {
			t.Fatal(e)
		}
		ctx := &zero.Ctx{Event: &event, State: zero.State{}}
		observeDocument(ctx)
		d, ok := currentDocument(documentScope(ctx), time.Now())
		if !ok || d.ID == "" || !documentExtension(d.Name) || d.Size != 512 {
			t.Fatal(d, ok)
		}
	}
}
func TestDocumentPermissionStopsBeforeIO(t *testing.T) {
	c := &http.Client{Transport: linkTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("network forbidden")
		return nil, context.Canceled
	})}
	if _, e := runDocumentQA(context.Background(), linkFixture(), ocrEvent("unused"), documentKey{456, 10, 123, ""}, goba.PermRoleUser, false, "总结", c, noOneBot(t)); e == nil {
		t.Fatal("permission")
	}
	cfg := linkFixture()
	cfg.API = ""
	if _, e := runDocumentQA(context.Background(), cfg, ocrEvent("unused"), documentKey{456, 10, 123, ""}, goba.PermRoleOwner, false, "总结", c, noOneBot(t)); e == nil {
		t.Fatal("model not ready")
	}
}
func TestDocumentMissingIDClearsPreviousFile(t *testing.T) {
	ctx := &zero.Ctx{Event: &zero.Event{SelfID: 456, UserID: 123, NativeMessage: []byte(`[{"type":"file","data":{"file":"new.pdf","file_size":"128"}}]`)}, State: zero.State{}}
	key := documentScope(ctx)
	rememberDocument(key, documentInput{ID: "old-id", Name: "old.pdf"}, time.Now())
	observeDocument(ctx)
	if _, ok := currentDocument(key, time.Now()); ok {
		t.Fatal("missing file_id retained stale file or used filename as ID")
	}
}
func TestDocumentCacheReplacementDoesNotEvictAnotherUser(t *testing.T) {
	documents.Lock()
	old := documents.items
	documents.items = map[documentKey]documentInput{}
	documents.Unlock()
	defer func() { documents.Lock(); documents.items = old; documents.Unlock() }()
	now := time.Now()
	for i := int64(1); i <= 128; i++ {
		rememberDocument(documentKey{456, 10, i, "cap"}, documentInput{ID: "old"}, now.Add(time.Duration(i)*time.Millisecond))
	}
	rememberDocument(documentKey{456, 10, 128, "cap"}, documentInput{ID: "new"}, now.Add(time.Second))
	if _, ok := currentDocument(documentKey{456, 10, 1, "cap"}, now.Add(time.Second)); !ok {
		t.Fatal("replacing own file evicted another user")
	}
}
func TestDocumentDownloadFailuresNoFurtherRequests(t *testing.T) {
	for _, tc := range []struct {
		status int
		size   int64
		body   string
	}{{302, 0, ""}, {200, documentLimit + 1, ""}, {200, 0, "invalid"}} {
		calls := 0
		c := &http.Client{Transport: linkTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Host != "files.qq.com" {
				t.Fatal("unexpected paid request")
			}
			h := make(http.Header)
			h.Set("Location", "https://evil.example/x")
			return &http.Response{StatusCode: tc.status, ContentLength: tc.size, Header: h, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})}
		call := func(context.Context, string, zero.Params) zero.APIResponse {
			return zero.APIResponse{Status: "ok", Data: gjson.Parse(`{"url":"https://files.qq.com/x"}`)}
		}
		if _, e := downloadDocument(context.Background(), c, documentKey{456, 10, 123, ""}, documentInput{ID: "x", Name: "x.pdf"}, call); e == nil || calls != 1 {
			t.Fatal(e, calls)
		}
	}
}
func TestDocumentLimits(t *testing.T) {
	var r ocrResult
	_ = json.Unmarshal([]byte(ocrOK), &r)
	r.PageCount = 101
	if _, e := documentText(r); e == nil {
		t.Fatal("page cap")
	}
	r.PageCount = 1
	r.Pages[0].Text = strings.Repeat("x", 120001)
	if _, e := documentText(r); e == nil {
		t.Fatal("text cap")
	}
}
