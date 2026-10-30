package query

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"time"

	"github.com/FloatTech/gg"
	"github.com/FloatTech/gg/factory"
	_ "golang.org/x/image/webp"
)

func danmakuAvatars(ctx context.Context, client *http.Client, addresses []string) []image.Image {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result := make([]image.Image, len(addresses))
	for i, address := range addresses {
		result[i] = danmakuAvatar(ctx, client, address)
	}
	return result
}

// Avatars are optional decoration; a failed lookup must not discard records.
func danmakuAvatar(ctx context.Context, client *http.Client, address string) image.Image {
	if address == "" || ctx.Err() != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || len(b) > 2<<20 {
		return nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 2048 || cfg.Height > 2048 {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	return factory.Size(img, 500, 500).Image()
}

func drawDanmakuAvatar(canvas *gg.Context, avatar image.Image, x, y int) {
	if avatar != nil {
		canvas.DrawImage(avatar, x, y)
		return
	}
	canvas.SetRGB255(238, 238, 238)
	canvas.DrawRectangle(float64(x), float64(y), 500, 500)
	canvas.Fill()
	canvas.SetRGB255(90, 90, 90)
	canvas.DrawString("头像暂不可用", float64(x+75), float64(y+250))
}
