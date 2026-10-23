package tarot

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/extension/rate"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var tarotLimits = rate.NewManager[int64](10*time.Second, 5)

func tarotLimit(ctx *zero.Ctx) *rate.Limiter {
	id := ctx.Event.GroupID
	if id == 0 {
		id = -ctx.Event.UserID
	}
	return tarotLimits.Load(id)
}

func tarotLimitNotice(ctx *zero.Ctx) {
	ctx.Break()
	ctx.SendChain(message.Text("塔罗牌请求过于频繁，请稍后再试。"))
}

func loadCardImage(ctx context.Context, folder, name string) ([]byte, error) {
	return fetchCardImage(ctx, folder, name, "https://raw.githubusercontent.com/FloatTech/zbpdata/main/Tarot/", http.DefaultClient)
}

// All cards in one command share a deadline. Missing art never blocks text output indefinitely.
func fetchCardImage(ctx context.Context, folder, name, base string, client *http.Client) ([]byte, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid card path")
	}
	filename := filepath.Join(folder, clean)
	valid := func(data []byte) bool { _, _, err := image.DecodeConfig(bytes.NewReader(data)); return err == nil }
	if data, err := os.ReadFile(filename); err == nil && valid(data) {
		return data, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+filepath.ToSlash(clean), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("card image status %d", resp.StatusCode)
	}
	const maxBytes = 8 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBytes || !valid(data) {
		return nil, fmt.Errorf("invalid card image")
	}
	// A cache-write failure should not discard an otherwise usable image.
	if os.MkdirAll(filepath.Dir(filename), 0755) == nil {
		if f, e := os.CreateTemp(filepath.Dir(filename), "card-*.tmp"); e == nil {
			tmp := f.Name()
			_, e = f.Write(data)
			closeErr := f.Close()
			if e == nil && closeErr == nil {
				_ = os.Rename(tmp, filename)
			}
			_ = os.Remove(tmp)
		}
	}
	return data, nil
}
