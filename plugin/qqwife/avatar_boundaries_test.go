package qqwife

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/wdvxdr1123/ZeroBot/message"
)

func TestAvatarStatusAndSizeBoundaries(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name         string
		status, size int
		valid        bool
	}{
		{"valid", 200, pngData.Len(), true},
		{"exact-limit", 200, 2 << 20, true},
		{"over-limit", 200, (2 << 20) + 1, false},
		{"bad-gateway", 502, pngData.Len(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := make([]byte, tc.size)
			copy(body, pngData.Bytes())
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); _, _ = w.Write(body) }))
			defer s.Close()
			got, err := fetchAvatar(context.Background(), s.URL, s.Client())
			if tc.valid {
				if err != nil || !bytes.Equal(got, body) {
					t.Fatal("valid response changed", err)
				}
			} else if err == nil || len(got) != 0 {
				t.Fatal("invalid response accepted")
			}
		})
	}
}

func TestAvatarFallbackAttemptBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name            string
		images, success bool
		calls           int
	}{
		{"image-success-once", true, true, 1},
		{"image-and-text-both-fail-stop", true, false, 2},
		{"text-success-once", false, true, 1},
		{"text-failure-no-repeat", false, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := message.Message{message.At(123), message.Text("已保存结果")}
			if tc.images {
				msg = append(msg, message.Image("base64://a"), message.Image("base64://b"))
			}
			before := append(message.Message(nil), msg...)
			calls := 0
			deliverWithImageFallback(msg, func(got message.Message) bool {
				calls++
				if calls == 1 && !reflect.DeepEqual(got, msg) {
					t.Fatal("initial message changed")
				}
				if calls == 2 {
					if len(got) != 3 || !reflect.DeepEqual(got[:2], msg[:2]) || got[2].Type != "text" {
						t.Fatal("fallback lost target/text or retained images")
					}
				}
				return tc.success
			})
			if calls != tc.calls || !reflect.DeepEqual(msg, before) {
				t.Fatal("unexpected retry or input mutation", calls)
			}
		})
	}
}
