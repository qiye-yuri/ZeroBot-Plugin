package updatewatch

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fakeFetcher(fn func(*http.Request) string) *Fetcher {
	return &Fetcher{Client: &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fn(r))), Header: make(http.Header)}, nil
	})}}
}

func TestOfficialVersionLogParsing(t *testing.T) {
	f := fakeFetcher(func(r *http.Request) string {
		if strings.HasSuffix(r.URL.Path, "/changelog") {
			return `{"status":"success","content":"## 1.4.0\nFixed dedicated server startup crash."}`
		}
		return `{"versions":[{"id":100566,"name":"1.4.0","type":"release","released":1792207502}]}`
	})
	events, e := f.ftb(context.Background())
	if e != nil || len(events) != 1 || events[0].Version != "1.4.0" || !strings.Contains(events[0].URLs[1], "100566/changelog") {
		t.Fatalf("%+v %v", events, e)
	}
}
func TestIssuesIgnoreOrdinaryReportsAndPlayerComments(t *testing.T) {
	f := fakeFetcher(func(r *http.Request) string {
		if strings.HasSuffix(r.URL.Path, "/comments") {
			return `[{"body":"unsupported player assertion","author_association":"NONE","html_url":"https://github.com/FTBTeam/FTB-Modpack-Issues/issues/2#issuecomment-1"},{"body":"Fixed recipe handling in development.","author_association":"MEMBER","html_url":"https://github.com/FTBTeam/FTB-Modpack-Issues/issues/2#issuecomment-2"}]`
		}
		return `[{"number":1,"title":"Player reported crash","labels":[{"name":"FTB Evolution 2"}]},{"number":2,"title":"Recipe issue","html_url":"https://github.com/FTBTeam/FTB-Modpack-Issues/issues/2","labels":[{"name":"FTB Evolution 2"},{"name":"state: Fixed in Dev"}]}]`
	})
	events, e := f.issues(context.Background())
	if e != nil || len(events) != 1 {
		t.Fatalf("%+v %v", events, e)
	}
	if strings.Contains(events[0].Text, "unsupported player") || !strings.Contains(events[0].Text, "Fixed recipe handling") || !strings.Contains(events[0].Stage, "未确认发布") {
		t.Fatal("unverified player report promoted to progress")
	}
}
func TestCurseForgeRejectsUnrecognizedMarkup(t *testing.T) {
	f := fakeFetcher(func(r *http.Request) string { return `<html>Please verify your browser</html>` })
	if _, e := f.curseforge(context.Background()); e == nil {
		t.Fatal("challenge page accepted as no updates")
	}
}
