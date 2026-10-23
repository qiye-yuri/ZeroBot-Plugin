package qqwife

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"time"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

func fetchAvatar(ctx context.Context, url string, client *http.Client) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("avatar status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 2<<20 {
		return nil, fmt.Errorf("avatar too large")
	}
	if _, _, err = image.DecodeConfig(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return data, nil
}

func withoutImages(msg message.Message) message.Message {
	plain := make(message.Message, 0, len(msg))
	for _, seg := range msg {
		if seg.Type != "image" {
			plain = append(plain, seg)
		}
	}
	return plain
}

func deliverWithImageFallback(msg message.Message, send func(message.Message) bool) {
	if send(msg) {
		return
	}
	plain := withoutImages(msg)
	if len(plain) != len(msg) {
		plain = append(plain, message.Text("\n（头像发送失败，以上为已保存的游戏结果）"))
		send(plain)
	}
}

// Download optional avatars ourselves, so OneBot cannot lose the entire result on URL failure.
func sendWithAvatarFallback(ctx *zero.Ctx, segments ...message.Segment) {
	deadline, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	msg := make(message.Message, 0, len(segments))
	missing := false
	for _, seg := range segments {
		if seg.Type == "image" && strings.HasPrefix(seg.Data["file"], "https://q4.qlogo.cn/") {
			data, err := fetchAvatar(deadline, seg.Data["file"], http.DefaultClient)
			if err != nil {
				missing = true
				continue
			}
			seg = message.ImageBytes(data)
		}
		msg = append(msg, seg)
	}
	if missing {
		msg = append(msg, message.Text("\n（头像暂不可用，游戏结果已保存）"))
	}
	deliverWithImageFallback(msg, func(m message.Message) bool { return ctx.Send(m).ID() != 0 })
}
