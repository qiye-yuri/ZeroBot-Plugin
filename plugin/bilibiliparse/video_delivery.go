package bilibiliparse

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wdvxdr1123/ZeroBot/message"
)

var videoSummaryClient = &http.Client{
	Timeout:       12 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// The already available card must reach QQ before optional upstream work starts.
func sendVideoResult(base []message.Segment, link string, summarize func() ([]message.Segment, error), send func([]message.Segment) bool) {
	if !send(base) || summarize == nil {
		return
	}
	summary, err := summarize()
	result := []message.Segment{message.Text("视频总结：", link, "\n")}
	if err != nil {
		result = append(result, message.Text("暂不可用：", err, "。基础视频信息已单独发送。"))
	} else {
		result = append(result, summary...)
	}
	send(result)
}

func requestVideoSummary(client *http.Client, endpoint, cookie string) ([]message.Segment, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("视频总结请求地址无效")
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", bilibiliparseReferer+"/")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("B站视频总结请求失败或超时")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("B站视频总结HTTP状态%d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("B站视频总结读取失败或超时")
	}
	if len(data) > 2<<20 {
		return nil, fmt.Errorf("B站视频总结响应超过2MiB")
	}
	return decodeVideoSummary(data)
}
