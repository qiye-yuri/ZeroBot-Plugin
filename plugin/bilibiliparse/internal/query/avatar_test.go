package query

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bz "github.com/FloatTech/AnimeAPI/bilibili"
	"github.com/FloatTech/gg"
	"golang.org/x/image/font/gofont/goregular"
)

func avatarPNG(t *testing.T, size int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestDanmakuReportWithUnavailableAvatars(t *testing.T) {
	font := os.Getenv("DANMAKU_AVATAR_FONT")
	data := goregular.TTF
	var err error
	if font != "" {
		data, err = os.ReadFile(font)
		if err != nil {
			t.Fatal(err)
		}
	}
	var records bz.Danmakusuki
	fixture := `{"code":200,"data":{"total":2,"pageNum":0,"data":{"records":[{"channel":{"uId":900001,"uName":"合成主播甲"},"live":{"title":"合成场次甲","isFinish":true,"danmakusCount":1,"startDate":1793332800000,"stopDate":1793336400000},"danmakus":[{"type":0,"sendDate":1793333400000,"message":"合成弹幕甲：头像失败仍保留"}]},{"channel":{"uId":900002,"uName":"合成主播乙"},"live":{"title":"合成场次乙","isFinish":true,"danmakusCount":1,"startDate":1793332800000,"stopDate":1793336400000},"danmakus":[{"type":0,"sendDate":1793334000000,"message":"合成弹幕乙：第二条记录仍可见"}]}]}}}`
	if err = json.Unmarshal([]byte(fixture), &records); err != nil {
		t.Fatal(err)
	}
	img, err := renderDanmakuReport(bz.MemberCard{Mid: "900000", Name: "合成测试用户"}, records, true, []image.Image{nil, nil, nil}, data)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dy() < 1600 || img.Bounds().Dx() < 1800 {
		t.Fatal("report lost record rows", img.Bounds())
	}
	var b bytes.Buffer
	if err = png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	if _, err = png.Decode(bytes.NewReader(b.Bytes())); err != nil {
		t.Fatal(err)
	}
	// Text remains on the right of the avatar slots in both record sections.
	for _, area := range []image.Rectangle{image.Rect(700, 550, 1900, 1000), image.Rect(700, 1250, 1900, img.Bounds().Dy())} {
		dark := 0
		for y := area.Min.Y; y < area.Max.Y; y++ {
			for x := area.Min.X; x < area.Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r < 30000 && g < 30000 && b < 30000 {
					dark++
				}
			}
		}
		if dark < 100 {
			t.Fatal("record text missing", area, dark)
		}
	}
	if output := os.Getenv("DANMAKU_REPORT_PREVIEW"); output != "" {
		if err = os.WriteFile(output, b.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDanmakuAvatarsKeepIndependentSlots(t *testing.T) {
	red, blue := avatarPNG(t, 16, color.RGBA{R: 255, A: 255}), avatarPNG(t, 16, color.RGBA{B: 255, A: 255})
	// Reuse the repository's synthetic solid-color WebP fixture (score tests).
	webp, err := base64.StdEncoding.DecodeString("UklGRi4AAABXRUJQVlA4TCIAAAAv/8F/AAdQ6dLWvP8BgUCyv/cMRfQ/4z//+c9//vOf//wf")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("avatar received credentials")
		}
		switch r.URL.Path {
		case "/red":
			w.Write(red)
		case "/bad.webp":
			http.Error(w, "missing", 404)
		case "/valid.webp":
			w.Write(webp)
		default:
			w.Write(blue)
		}
	}))
	defer srv.Close()
	avatars := danmakuAvatars(context.Background(), srv.Client(), []string{srv.URL + "/red", srv.URL + "/bad.webp", srv.URL + "/blue", srv.URL + "/valid.webp"})
	if len(avatars) != 4 || avatars[0] == nil || avatars[1] != nil || avatars[2] == nil || avatars[3] == nil || calls.Load() != 4 {
		t.Fatal("optional failure aborted or reused another avatar", calls.Load())
	}
	canvas := gg.NewContext(2000, 600)
	canvas.SetRGB255(255, 255, 255)
	canvas.Clear()
	if font := os.Getenv("DANMAKU_AVATAR_FONT"); font != "" {
		if err := canvas.LoadFontFace(font, 50); err != nil {
			t.Fatal(err)
		}
	}
	for i, avatar := range avatars {
		drawDanmakuAvatar(canvas, avatar, i*500, 0)
	}
	canvas.SetRGB255(0, 0, 0)
	canvas.DrawString("synthetic records remain", 20, 550)
	for _, tc := range []struct {
		x    int
		want color.RGBA
	}{{20, color.RGBA{R: 255, A: 255}}, {520, color.RGBA{R: 238, G: 238, B: 238, A: 255}}, {1020, color.RGBA{B: 255, A: 255}}} {
		got := color.RGBAModel.Convert(canvas.Image().At(tc.x, 20)).(color.RGBA)
		if got != tc.want {
			t.Fatal(tc.x, got, tc.want)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas.Image()); err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(encoded.Bytes())); err != nil {
		t.Fatal("report not renderable after avatar failure", err)
	}
	if output := os.Getenv("DANMAKU_AVATAR_PREVIEW"); output != "" {
		if err := os.WriteFile(output, encoded.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDanmakuAvatarFailuresShareDeadline(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); <-r.Context().Done() }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	avatars := danmakuAvatars(ctx, srv.Client(), []string{srv.URL, srv.URL, srv.URL})
	if calls.Load() != 1 {
		t.Fatal("later avatars retried after deadline", calls.Load())
	}
	for _, a := range avatars {
		if a != nil {
			t.Fatal("timeout has an avatar")
		}
	}
}

func TestDanmakuAvatarRejectsInvalidAndOversizedImages(t *testing.T) {
	large := avatarPNG(t, 2049, color.White)
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"invalid", []byte("not an image")},
		{"oversized-body", []byte(strings.Repeat("x", (2<<20)+1))},
		{"oversized-dimensions", large},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(tc.body) }))
			defer srv.Close()
			if danmakuAvatar(context.Background(), srv.Client(), srv.URL) != nil {
				t.Fatal("bad avatar accepted")
			}
		})
	}
}
