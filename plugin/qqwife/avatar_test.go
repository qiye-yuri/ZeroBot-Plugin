package qqwife

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wdvxdr1123/ZeroBot/message"
)

func TestAvatarTimeoutAndInvalidResponse(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte("not an image"))
	}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := fetchAvatar(ctx, s.URL+"/slow", s.Client()); err == nil {
		t.Fatal("timeout ignored")
	}
	if time.Since(start) > time.Second {
		t.Fatal("avatar blocked too long")
	}
	if _, err := fetchAvatar(context.Background(), s.URL, s.Client()); err == nil {
		t.Fatal("invalid avatar accepted")
	}
}

func TestFailedImageSendKeepsSavedResultText(t *testing.T) {
	msg := message.Message{message.At(123), message.Text("已保存配对结果"), message.Image("base64://test")}
	calls := 0
	deliverWithImageFallback(msg, func(got message.Message) bool {
		calls++
		if calls == 1 {
			return false
		}
		if got[0].Type != "at" || got[1].Data["text"] != "已保存配对结果" {
			t.Fatal("lost recipient or result")
		}
		for _, seg := range got {
			if seg.Type == "image" {
				t.Fatal("fallback retried image")
			}
		}
		return true
	})
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}
