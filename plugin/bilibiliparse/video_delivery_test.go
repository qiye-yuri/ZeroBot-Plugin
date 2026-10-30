package bilibiliparse

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wdvxdr1123/ZeroBot/message"
)

func TestVideoCardPrecedesBlockedSummary(t *testing.T) {
	sent := make(chan string, 2)
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		sendVideoResult([]message.Segment{message.Text("BASE_CARD")}, "https://example.invalid/video", func() ([]message.Segment, error) {
			close(started)
			<-release
			return []message.Segment{message.Text("SUMMARY")}, nil
		}, func(m []message.Segment) bool { sent <- fmt.Sprint(m); return true })
	}()
	defer func() {
		close(release)
		<-done
		select {
		case second := <-sent:
			if !strings.Contains(second, "SUMMARY") || !strings.Contains(second, "https://example.invalid/video") {
				t.Fatal(second)
			}
		default:
			t.Fatal("summary result missing")
		}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("summary did not start")
	}
	select {
	case first := <-sent:
		if !strings.Contains(first, "BASE_CARD") {
			t.Fatal(first)
		}
	default:
		t.Fatal("base card withheld until summary finishes")
	}
}

func TestVideoDeliveryFailureDisabledAndEmptySummary(t *testing.T) {
	for _, tc := range []struct {
		name      string
		summarize func() ([]message.Segment, error)
		accepted  bool
		want      string
		count     int
	}{
		{"disabled", nil, true, "", 1},
		{"failure", func() ([]message.Segment, error) { return nil, fmt.Errorf("业务码-1") }, true, "暂不可用", 2},
		{"empty", func() ([]message.Segment, error) {
			return decodeVideoSummary([]byte(`{"code":0,"data":{"model_result":{"summary":""}}}`))
		}, true, "暂无可用", 2},
		{"base-send-failed", func() ([]message.Segment, error) {
			t.Fatal("summary requested after failed base send")
			return nil, nil
		}, false, "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var messages []string
			sendVideoResult([]message.Segment{message.Text("BASE_CARD")}, "https://example.invalid/video", tc.summarize, func(m []message.Segment) bool { messages = append(messages, fmt.Sprint(m)); return tc.accepted })
			if len(messages) != tc.count || !strings.Contains(messages[0], "BASE_CARD") {
				t.Fatal(messages)
			}
			if tc.count == 2 && (!strings.Contains(messages[1], tc.want) || !strings.Contains(messages[1], "https://example.invalid/video") || strings.Contains(messages[1], "BASE_CARD")) {
				t.Fatal(messages)
			}
		})
	}
}

func TestVideoSummaryHTTPBoundaries(t *testing.T) {
	var calls, redirected atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Cookie") != "synthetic-cookie" || r.Header.Get("User-Agent") == "" {
			t.Error("request headers missing")
		}
		switch r.URL.Path {
		case "/slow":
			<-r.Context().Done()
		case "/redirect":
			http.Redirect(w, r, destination.URL, http.StatusFound)
		case "/large":
			fmt.Fprint(w, strings.Repeat("x", (2<<20)+1))
		case "/failure":
			http.Error(w, "upstream unavailable", 503)
		default:
			fmt.Fprint(w, `{"code":0,"data":{"model_result":{"summary":"synthetic summary"}}}`)
		}
	}))
	defer server.Close()
	for _, route := range []string{"/ok", "/slow", "/redirect", "/large", "/failure"} {
		t.Run(route, func(t *testing.T) {
			client := *videoSummaryClient
			if route == "/slow" {
				client.Timeout = 50 * time.Millisecond
			}
			before := calls.Load()
			msg, err := requestVideoSummary(&client, server.URL+route, "synthetic-cookie")
			if route == "/ok" {
				if err != nil || !strings.Contains(fmt.Sprint(msg), "synthetic summary") {
					t.Fatal(msg, err)
				}
			} else if err == nil {
				t.Fatal("failed summary accepted", route)
			}
			if calls.Load() != before+1 {
				t.Fatal("request retried")
			}
		})
	}
	if redirected.Load() != 0 {
		t.Fatal("cookie request followed redirect")
	}
}
