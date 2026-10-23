package query

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDanmakuTLSVerification(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := newDanmakuClient()
	defer client.CloseIdleConnections()
	if response, err := client.Get(server.URL); err == nil {
		response.Body.Close()
		t.Fatal("untrusted certificate was accepted")
	} else {
		var verificationError *tls.CertificateVerificationError
		if !errors.As(err, &verificationError) {
			t.Fatalf("expected certificate rejection, got: %v", err)
		}
	}
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{RootCAs: roots}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("trusted certificate rejected: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.StatusCode)
	}
}

func TestQueryResponses(t *testing.T) {
	cases := []struct {
		name, body, want string
		status           int
		member           bool
	}{
		{"member-empty", "", "完整JSON", 200, true},
		{"member-truncated", `{"code":0`, "完整JSON", 200, true},
		{"member-html", `<html>busy</html>`, "完整JSON", 200, true},
		{"member-rate-limit", `{"code":-412}`, "code=-412", 200, true},
		{"member-missing", `{"code":0,"data":{}}`, "缺少有效用户资料", 200, true},
		{"member-null", `{"code":0,"data":{"card":null}}`, "缺少有效用户资料", 200, true},
		{"member-success", `{"code":0,"data":{"card":{"mid":"123","name":"fixture"}}}`, "", 200, true},
		{"member-no-code", `{"data":{"card":{"mid":"123","name":"fixture"}}}`, "请求未成功", 200, true},
		{"danmaku-empty", "", "完整JSON", 200, false},
		{"danmaku-truncated", `{"code":200`, "完整JSON", 200, false},
		{"danmaku-api-error", `{"code":500,"data":{}}`, "code=500", 200, false},
		{"danmaku-http-error", `{"code":200}`, "HTTP 503", 503, false},
		{"danmaku-no-records-key", `{"code":200,"data":{}}`, "缺少记录列表", 200, false},
		{"danmaku-wrong-records", `{"code":200,"data":{"data":{"records":{}}}}`, "缺少记录列表", 200, false},
		{"danmaku-valid-empty", `{"code":200,"data":{"data":{"records":[]}}}`, "", 200, false},
		{"danmaku-valid-record", `{"code":200,"data":{"data":{"records":[{"channel":{"uId":123}}]}}}`, "", 200, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			client := newDanmakuClient()
			defer client.CloseIdleConnections()
			var err error
			if tc.member {
				_, err = memberCard(context.Background(), client, srv.URL)
			} else {
				_, err = getDanmaku(context.Background(), client, srv.URL)
			}
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v want %s", err, tc.want)
			}
			wantCalls := 1
			if strings.Contains(tc.want, "完整JSON") {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Fatalf("calls=%d want=%d", calls, wantCalls)
			}
		})
	}
}

func TestQueryTransientJSONAndDeadline(t *testing.T) {
	t.Run("retry-once", func(t *testing.T) {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				return
			}
			w.Write([]byte(`{"code":200,"data":{"data":{"records":[]}}}`))
		}))
		defer srv.Close()
		client := newDanmakuClient()
		defer client.CloseIdleConnections()
		if _, err := getDanmaku(context.Background(), client, srv.URL); err != nil {
			t.Fatal(err)
		}
		if calls != 2 {
			t.Fatalf("calls=%d", calls)
		}
	})
	t.Run("deadline", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
		defer srv.Close()
		client := newDanmakuClient()
		defer client.CloseIdleConnections()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := getDanmaku(ctx, client, srv.URL)
		if err == nil || time.Since(start) > time.Second {
			t.Fatalf("deadline not respected: %v", err)
		}
	})
	t.Run("oversize", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat("x", (2<<20)+1))) }))
		defer srv.Close()
		client := newDanmakuClient()
		defer client.CloseIdleConnections()
		_, err := getDanmaku(context.Background(), client, srv.URL)
		if err == nil || !strings.Contains(err.Error(), "响应过大") {
			t.Fatalf("got %v", err)
		}
	})
}
