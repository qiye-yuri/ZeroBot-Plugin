package query

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	bz "github.com/FloatTech/AnimeAPI/bilibili"
	"github.com/tidwall/gjson"
)

func newDanmakuClient() *http.Client {
	return &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DisableKeepAlives: true}}
}

// Retry an empty/truncated JSON response once within the caller's deadline.
// Business errors and HTTP rejection are not retried.
func queryJSON(ctx context.Context, client *http.Client, url, source string) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("%s：请求地址无效", source)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s：请求失败或超时", source)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s：HTTP %d", source, resp.StatusCode)
		}
		if len(data) > 2<<20 {
			return nil, fmt.Errorf("%s：响应过大", source)
		}
		if readErr == nil && json.Valid(data) && bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
			return data, nil
		}
		if attempt == 1 {
			return nil, fmt.Errorf("%s：响应为空或不是完整JSON，请稍后重试", source)
		}
	}
	panic("unreachable")
}

func memberCard(ctx context.Context, client *http.Client, url string) (bz.MemberCard, error) {
	var card bz.MemberCard
	data, err := queryJSON(ctx, client, url, "B站用户资料接口")
	if err != nil {
		return card, err
	}
	root := gjson.ParseBytes(data)
	code := root.Get("code")
	if code.Type != gjson.Number || code.Int() != 0 {
		return card, fmt.Errorf("B站用户资料接口：请求未成功（code=%s）", code.Raw)
	}
	raw := root.Get("data.card")
	if !raw.IsObject() || json.Unmarshal([]byte(raw.Raw), &card) != nil || card.Mid == "" || card.Name == "" {
		return card, fmt.Errorf("B站用户资料接口：缺少有效用户资料")
	}
	return card, nil
}

func getMemberCard(id string) (bz.MemberCard, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	client := newDanmakuClient()
	defer client.CloseIdleConnections()
	return memberCard(ctx, client, fmt.Sprintf(bz.MemberCardURL, id))
}

func getDanmaku(ctx context.Context, client *http.Client, url string) (bz.Danmakusuki, error) {
	var result bz.Danmakusuki
	data, err := queryJSON(ctx, client, url, "弹幕记录接口")
	if err != nil {
		return result, err
	}
	root := gjson.ParseBytes(data)
	code := root.Get("code")
	if code.Type != gjson.Number || code.Int() != 200 {
		return result, fmt.Errorf("弹幕记录接口：请求未成功（code=%s）", code.Raw)
	}
	if !root.Get("data.data.records").IsArray() {
		return result, fmt.Errorf("弹幕记录接口：缺少记录列表")
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("弹幕记录接口：数据格式不正确")
	}
	return result, nil
}
