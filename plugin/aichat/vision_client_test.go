package aichat

import (
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FloatTech/ZeroBot-Plugin/internal/modelstats"
)

type visionTestTransport struct {
	body   *visionTestBody
	length int64
	status int
	closed bool
}

func (t *visionTestTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: t.status, ContentLength: t.length, Body: t.body, Header: make(http.Header)}, nil
}
func (t *visionTestTransport) CloseIdleConnections() { t.closed = true }

type visionTestBody struct {
	io.Reader
	read, closes int
}

func (b *visionTestBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (b *visionTestBody) Close() error { b.closes++; return nil }

func TestVisionResponseBoundsCloseAndStatistics(t *testing.T) {
	for _, tc := range []struct {
		name         string
		length       int64
		size, status int
		failed       bool
	}{{"declared", visionResponseBytes + 1, visionResponseBytes + 1, 200, true}, {"exact", -1, visionResponseBytes, 200, false}, {"streamed", -1, visionResponseBytes + 1, 200, true}, {"error-body", -1, visionResponseBytes + 1, 500, true}, {"empty", 0, 0, 200, false}} {
		t.Run(tc.name, func(t *testing.T) {
			body := &visionTestBody{Reader: strings.NewReader(strings.Repeat("x", tc.size))}
			transport := &visionTestTransport{body: body, length: tc.length, status: tc.status}
			collector := modelstats.New()
			client := collector.Client(boundedVisionClient(&http.Client{Transport: transport}), modelstats.Vision)
			rsp, err := client.Get("https://synthetic.invalid/vision")
			var data []byte
			if tc.failed {
				if !errors.Is(err, errVisionResponseLimit) || body.read > visionResponseBytes+1 || (tc.name == "declared" && body.read != 0) {
					t.Fatal("excess not bounded", err, body.read)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				data, err = io.ReadAll(rsp.Body)
				if err != nil || len(data) != tc.size {
					t.Fatal(err, len(data))
				}
				rsp.Body.Close()
			}
			if body.closes != 1 {
				t.Fatal("response body not closed", body.closes)
			}
			stats := collector.Read()
			if stats.Started != 1 || stats.Active != 0 || stats.Incomplete != 0 || (tc.failed && (stats.Failed != 1 || stats.Completed != 0)) || (!tc.failed && (stats.Completed != 1 || stats.Failed != 0)) {
				t.Fatal("size rejection misclassified", stats.Counts)
			}
			client.CloseIdleConnections()
			if !transport.closed {
				t.Fatal("idle close not forwarded")
			}
		})
	}
}

func TestVisionResponseGzipRedirectAndClientSettings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/valid", 302)
		case "/valid":
			w.Write([]byte("valid"))
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			io.Copy(gz, strings.NewReader(strings.Repeat("x", visionResponseBytes+1)))
			gz.Close()
		case "/chunked":
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			io.Copy(w, strings.NewReader(strings.Repeat("x", visionResponseBytes+1)))
		}
	}))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	redirects := 0
	base := &http.Client{Timeout: time.Second, Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { redirects++; return nil }}
	client := boundedVisionClient(base)
	if client == base || client.Timeout != base.Timeout || client.Jar != jar || base.Transport != nil {
		t.Fatal("client settings altered")
	}
	rsp, err := client.Get(server.URL + "/redirect")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(rsp.Body)
	rsp.Body.Close()
	if err != nil || string(data) != "valid" || redirects != 1 {
		t.Fatal("redirect policy lost", err)
	}
	for _, path := range []string{"/gzip", "/chunked"} {
		_, err = client.Get(server.URL + path)
		if !errors.Is(err, errVisionResponseLimit) {
			t.Fatal("decoded response bypassed limit", path, err, len(data))
		}
	}
	if boundedVisionClient(nil) == nil {
		t.Fatal("default client missing")
	}
}
