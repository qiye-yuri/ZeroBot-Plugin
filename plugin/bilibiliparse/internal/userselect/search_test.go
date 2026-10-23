package userselect

import (
	bz "github.com/FloatTech/AnimeAPI/bilibili"
	"io"
	"net/http"
	"strings"
	"testing"
)

type searchTransport func(*http.Request) (*http.Response, error)

func (f searchTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSearchResponses(t *testing.T) {
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`{"code":0,"data":{"result":[{"mid":123}]}}`, true},
		{`{"code":0,"data":{"result":[]}}`, true},
		{`{"code":-412}`, false}, {``, false}, {`{}`, false}, {`{"code":0,"data":{}}`, false},
		{`{"code":0,"data":{"result":[{"mid":0}]}}`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			client := &http.Client{Transport: searchTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Scheme != "https" || r.URL.Host != "api.bilibili.com" || r.URL.Query().Get("keyword") != "测试 &名字" || r.Header.Get("Cookie") != "SESSDATA=fixture" {
					t.Fatal("request mismatch")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			_, err := searchWithClient(client, "测试 &名字", "SESSDATA=fixture")
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
func TestCookieSearchRejectsRedirect(t *testing.T) {
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	calls := 0
	http.DefaultTransport = searchTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"http://other.invalid/"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	cfg := bz.NewCookieConfig("unused")
	cfg.BilibiliCookie = "SESSDATA=fixture"
	if _, err := searchUser(cfg, "fixture"); err == nil {
		t.Fatal("redirect accepted")
	}
	if calls != 1 {
		t.Fatalf("redirect followed: %d", calls)
	}
}
