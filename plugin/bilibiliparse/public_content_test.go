package bilibiliparse

import (
	"context"
	"errors"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func TestOpusLinks(t *testing.T) {
	for _, link := range []string{"https://www.bilibili.com/opus/1257597472619888646 有效专栏", "https://bilibili.com/opus/123?from=share", `https:\/\/www.bilibili.com\/opus\/123`, "t.bilibili.com/123", "https://m.bilibili.com/dynamic/123"} {
		m := searchDynamicRe.FindStringSubmatch(link)
		if len(m) != 3 || m[2] == "" {
			t.Fatal(link, m)
		}
	}
	if searchDynamicRe.MatchString("https://www.bilibili.com/1257597472619888646") {
		t.Fatal("bare number is not a content type")
	}
}
func TestOptionalCookie(t *testing.T) {
	for _, tc := range []struct {
		name, cookie string
		err          error
		wantErr      bool
	}{{"missing", "", errors.New("no cookie config"), false}, {"existing", "synthetic=1", nil, false}, {"corrupt", "", errors.New("malformed configuration"), true}} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := optionalBiliCookie(func() (string, error) { return tc.cookie, tc.err })
			if (err != nil) != tc.wantErr || got != tc.cookie {
				t.Fatal(got, err)
			}
		})
	}
}
func TestPublicBiliResponseErrors(t *testing.T) {
	old := publicBiliClient
	defer func() { publicBiliClient = old }()
	for _, body := range []string{`{"code":-352,"message":"limited"}`, `<html>failure</html>`, `{"data":{}}`} {
		publicBiliClient = &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Cookie") != "" {
				t.Fatal("unexpected credentials")
			}
			return fixtureResponse(body), nil
		})}
		if _, err := publicBiliJSON(context.Background(), "https://api.bilibili.com/test", ""); err == nil {
			t.Fatal("accepted", body)
		}
	}
}
func TestPublicContent(t *testing.T) {
	old := publicBiliClient
	defer func() { publicBiliClient = old }()
	profileFails := false
	publicBiliClient = &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Cookie") != "" {
			t.Fatal("unexpected credentials")
		}
		switch r.URL.Path {
		case "/x/polymer/web-dynamic/v1/detail":
			return fixtureResponse(`{"code":0,"data":{"item":{"modules":{"module_author":{"name":"合成作者"},"module_dynamic":{"major":{"type":"MAJOR_TYPE_ARTICLE","article":{"title":"合成专栏","desc":"内容摘要","covers":["https://example.org/a.png"]}}}}}}}`), nil
		case "/room/v1/Room/get_info":
			return fixtureResponse(`{"code":0,"data":{"uid":123,"room_id":456,"title":"合成直播间","live_status":0,"area_name":"游戏","user_cover":"https://example.org/live.png"}}`), nil
		default:
			if profileFails {
				return fixtureResponse(`{"code":-352}`), nil
			}
			return fixtureResponse(`{"code":0,"data":{"info":{"uname":"合成主播"}}}`), nil
		}
	})}
	msg, err := publicDynamicMessage("123", "")
	if err != nil || !strings.Contains(msg.String(), "合成专栏") || !strings.Contains(msg.String(), "合成作者") || len(msg) != 2 {
		t.Fatal(msg, err)
	}
	msg, err = publicLiveMessage("456", "")
	if err != nil || !strings.Contains(msg.String(), "合成主播") || !strings.Contains(msg.String(), "未开播") {
		t.Fatal(msg, err)
	}
	profileFails = true
	msg, err = publicLiveMessage("456", "")
	if err != nil || !strings.Contains(msg.String(), "UID 123") {
		t.Fatal("profile fallback", msg, err)
	}
}
func TestOpusBodyAndMissingContent(t *testing.T) {
	msg, err := formatPublicDynamic(gjson.Parse(`{"modules":{"module_author":{"name":"作者"},"module_dynamic":{"major":{"type":"MAJOR_TYPE_OPUS","opus":{"title":"新动态","summary":{"text":"正文"},"pics":[{"url":"https://example.org/a.jpg"}]}}}}}`), "123")
	if err != nil || !strings.Contains(msg.String(), "新动态") || !strings.Contains(msg.String(), "正文") || len(msg) != 2 {
		t.Fatal(msg, err)
	}
	if _, err = formatPublicDynamic(gjson.Parse(`{}`), "123"); err == nil {
		t.Fatal("missing content accepted")
	}
}

func TestPublicPageFallback(t *testing.T) {
	page := `<script>window.__INITIAL_STATE__={"detail":{"id_str":"123","modules":[{"module_type":"MODULE_TYPE_TITLE","module_title":{"text":"公开标题"}},{"module_type":"MODULE_TYPE_AUTHOR","module_author":{"name":"公开作者"}},{"module_type":"MODULE_TYPE_CONTENT","module_content":{"paragraphs":[{"text":{"nodes":[{"word":{"words":"公开正文"}}]},"pic":{"pics":[{"url":"https://example.org/a.png"}]}}]}}]}};(function(){throw "must not execute"})()</script>`
	msg, err := parseOpusPage(page, "123")
	if err != nil || !strings.Contains(msg.String(), "公开标题") || !strings.Contains(msg.String(), "公开正文") || len(msg) != 2 {
		t.Fatal(msg, err)
	}
	for _, bad := range []string{`<html>login required</html>`, `window.__INITIAL_STATE__=not-json`, strings.Replace(page, `"id_str":"123"`, `"id_str":"456"`, 1)} {
		if _, err := parseOpusPage(bad, "123"); err == nil {
			t.Fatal("accepted non-content page")
		}
	}
	old := publicBiliClient
	defer func() { publicBiliClient = old }()
	publicBiliClient = &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.bilibili.com" {
			return fixtureResponse(`{"code":-352}`), nil
		}
		return fixtureResponse(page), nil
	})}
	msg, err = publicDynamicMessage("123", "")
	if err != nil || !strings.Contains(msg.String(), "公开标题") {
		t.Fatal("fallback failed", msg, err)
	}
}

func TestLegacyArticle(t *testing.T) {
	old := publicBiliClient
	defer func() { publicBiliClient = old }()
	for _, tc := range []struct {
		name, api, final string
		good             bool
	}{
		{"api", `{"code":0,"data":{"title":"合成专栏","author_name":"作者","origin_image_urls":["https://example.org/a.jpg"]}}`, "", true},
		{"redirect", `{"code":-352}`, "https://www.bilibili.com/opus/123", true},
		{"empty", `{"code":0,"data":{}}`, "https://www.bilibili.com/read/cv456", false},
		{"foreign", `{"code":-352}`, "https://example.org/opus/123", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			publicBiliClient = &http.Client{Transport: fixtureTransport(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Cookie") != "" {
					t.Fatal("unexpected credentials")
				}
				if r.URL.Host == "api.bilibili.com" {
					return fixtureResponse(tc.api), nil
				}
				resp := fixtureResponse(`window.__INITIAL_STATE__={"detail":{"id_str":"123","modules":[{"module_type":"MODULE_TYPE_TITLE","module_title":{"text":"合成专栏"}},{"module_type":"MODULE_TYPE_AUTHOR","module_author":{"name":"作者"}}]}}`)
				target, _ := http.NewRequest("GET", tc.final, nil)
				resp.Request = target
				return resp, nil
			})}
			msg, err := publicArticleMessage("456", "")
			if (err == nil) != tc.good {
				t.Fatal(msg, err)
			}
			if tc.good && !strings.Contains(msg.String(), "合成专栏") {
				t.Fatal(msg)
			}
		})
	}
}
