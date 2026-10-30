package aiimage

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImageAPIErrorDetailsAndRedaction(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("x-request-id", "req_synthetic_400")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "Invalid images: synthetic-key private prompt data:image/png;base64,YWJj https://example.com/?key=secret [CQ:at,qq=123]", "type": "invalid_request_error", "code": "invalid_image", "param": "images[4]"}, "raw_body": "DO_NOT_EXPOSE"})
	}))
	defer server.Close()
	_, err := generateImage(server.Client(), imageConfig{APIURL: server.URL, APIKey: "synthetic-key"}, "openai", "private prompt")
	if err == nil {
		t.Fatal("missing error")
	}
	s := err.Error()
	for _, v := range []string{"HTTP 400", "Invalid images", "invalid_request_error", "invalid_image", "images［4］", "req_synthetic_400"} {
		if !strings.Contains(s, v) {
			t.Fatal("missing detail", v, s)
		}
	}
	for _, v := range []string{"synthetic-key", "private prompt", "YWJj", "https://", "[CQ:", "DO_NOT_EXPOSE"} {
		if strings.Contains(s, v) {
			t.Fatal("leaked", v)
		}
	}
	if calls != 1 {
		t.Fatal("automatic retry", calls)
	}
}

func TestImageAPIErrorFallbackAndBounds(t *testing.T) {
	for _, body := range []string{"<html>secret gateway</html>", `{"error":`, strings.Repeat("x", maxImageErrorBody+1), `{"error":{"message":null,"code":null,"param":null}}`} {
		resp := &http.Response{StatusCode: 502, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
		s := imageAPIError(resp, imageConfig{}, "", nil).Error()
		if !strings.Contains(s, "HTTP 502") || !strings.Contains(s, "未提供") || strings.Contains(s, "secret") || len(s) > 1000 {
			t.Fatal(s)
		}
	}
	body, _ := json.Marshal(map[string]any{"error": map[string]any{"message": strings.Repeat("错", 2000), "code": 42}})
	s := imageAPIError(&http.Response{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, imageConfig{}, "", nil).Error()
	if !strings.Contains(s, "…") || !strings.Contains(s, "42") || len([]rune(s)) > 800 {
		t.Fatal("unbounded error")
	}
}

func TestImageAPIErrorNumericCodePreservesExactValue(t *testing.T) {
	for _, code := range []string{"9007199254740993", "1e+30", "0"} {
		body := `{"error":{"code":` + code + `}}`
		got := imageAPIError(&http.Response{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, imageConfig{}, "", nil).Error()
		if !strings.Contains(got, "错误码: "+code) {
			t.Fatalf("provider code changed: %s -> %s", code, got)
		}
	}
}
