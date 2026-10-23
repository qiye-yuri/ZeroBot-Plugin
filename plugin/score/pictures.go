package score

import (
	"bytes"
	"context"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Decode actual bytes, not a URL suffix or Content-Type, then store a real PNG.
func normalizePicture(data []byte, background bool) ([]byte, error) {
	if len(data) > 8<<20 {
		return nil, fmt.Errorf("image exceeds 8 MiB")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 20000000 {
		return nil, fmt.Errorf("invalid image dimensions")
	}
	if background && (cfg.Width < 400 || cfg.Height < 400) {
		return nil, fmt.Errorf("background is too small")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	err = png.Encode(&b, img)
	return b.Bytes(), err
}
func fallbackPicture(background bool) []byte {
	w, h := 256, 256
	if background {
		w, h = 1280, 720
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(190 + 40*y/h), uint8(205 + 30*x/w), 235, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
func fetchPicture(ctx context.Context, url string, background bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ZeroBot-Signin")
	req.Header.Set("Referer", referer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("image status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
	if err != nil {
		return nil, err
	}
	return normalizePicture(data, background)
}
func writePicture(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".picture-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}
func preparePictures(path, avatarURL, bgURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	avatar, err := fetchPicture(ctx, avatarURL, false)
	if err != nil {
		avatar = fallbackPicture(false)
	}
	raw, err := os.ReadFile(path)
	var background []byte
	if err == nil {
		background, err = normalizePicture(raw, true)
	}
	if err != nil {
		background, err = fetchPicture(ctx, bgURL, true)
	}
	if err != nil {
		background = fallbackPicture(true)
	}
	return avatar, writePicture(path, background)
}
func initPic(path string, uid int64) ([]byte, error) {
	return preparePictures(path, "https://q4.qlogo.cn/g?b=qq&nk="+strconv.FormatInt(uid, 10)+"&s=640", backgroundURL)
}
