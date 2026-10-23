package bilibiliparse

import (
	"context"
	"encoding/json"
	"fmt"
	bz "github.com/FloatTech/AnimeAPI/bilibili"
	"github.com/tidwall/gjson"
	"github.com/wdvxdr1123/ZeroBot/message"
	"io"
	"net/http"
	"strings"
	"time"
)

var publicBiliClient = &http.Client{Timeout: 12 * time.Second}

func optionalBiliCookie(load func() (string, error)) (string, error) {
	cookie, err := load()
	if err != nil && err.Error() == "no cookie config" {
		return "", nil
	}
	return cookie, err
}

func publicBiliJSON(ctx context.Context, endpoint, cookie string) (gjson.Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return gjson.Result{}, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", bilibiliparseReferer+"/")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := publicBiliClient.Do(req)
	if err != nil {
		return gjson.Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return gjson.Result{}, fmt.Errorf("B站接口HTTP状态%d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20+1))
	if err != nil {
		return gjson.Result{}, err
	}
	if len(data) > 2<<20 || !gjson.ValidBytes(data) {
		return gjson.Result{}, fmt.Errorf("B站接口返回无效数据")
	}
	result := gjson.ParseBytes(data)
	if !result.Get("code").Exists() {
		return gjson.Result{}, fmt.Errorf("B站接口缺少状态码")
	}
	if result.Get("code").Int() != 0 {
		return gjson.Result{}, fmt.Errorf("B站接口错误（%d）：%s；可能涉及内容权限、登录状态或访问限制", result.Get("code").Int(), result.Get("message").String())
	}
	return result.Get("data"), nil
}

func clippedText(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
func addPublicImage(msg message.Message, raw string) message.Message {
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") {
		return append(msg, message.Image(raw))
	}
	return msg
}

func publicDynamicMessage(id, cookie string) (message.Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	data, err := publicBiliJSON(ctx, "https://api.bilibili.com/x/polymer/web-dynamic/v1/detail?id="+id, cookie)
	if err != nil {
		msg, pageErr := publicOpusPage(ctx, id, cookie)
		if pageErr == nil {
			return msg, nil
		}
		return nil, fmt.Errorf("%v；公开页面读取失败: %w", err, pageErr)
	}
	return formatPublicDynamic(data.Get("item"), id)
}

func publicOpusPage(ctx context.Context, id, cookie string) (message.Message, error) {
	return publicOpusURL(ctx, "https://www.bilibili.com/opus/"+id, id, cookie)
}
func publicOpusURL(ctx context.Context, pageURL, id, cookie string) (message.Message, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", bilibiliparseReferer+"/")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := publicBiliClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP状态%d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 2<<20 {
		return nil, fmt.Errorf("公开页面过大")
	}
	if id == "" && resp.Request != nil {
		u := resp.Request.URL
		if u.Hostname() == "www.bilibili.com" || u.Hostname() == "bilibili.com" {
			m := searchDynamicRe.FindStringSubmatch(u.String())
			if len(m) == 3 {
				id = m[2]
			}
		}
	}
	if id == "" {
		return nil, fmt.Errorf("旧专栏页面未跳转到可读取的opus内容")
	}
	return parseOpusPage(string(data), id)
}

// Decode the embedded JSON only. Never execute page scripts or validation challenges.
func parseOpusPage(page, id string) (message.Message, error) {
	const marker = "window.__INITIAL_STATE__="
	_, tail, ok := strings.Cut(page, marker)
	if !ok {
		return nil, fmt.Errorf("页面未提供公开内容，可能需要登录或验证")
	}
	var raw json.RawMessage
	if err := json.NewDecoder(strings.NewReader(tail)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("页面数据格式不支持")
	}
	state := gjson.ParseBytes(raw)
	detail := state.Get("detail")
	if detail.Get("id_str").String() != id {
		return nil, fmt.Errorf("页面内容ID不匹配")
	}
	title, author, body := "", "", ""
	var images []string
	for _, m := range detail.Get("modules").Array() {
		switch m.Get("module_type").String() {
		case "MODULE_TYPE_TITLE":
			title = m.Get("module_title.text").String()
		case "MODULE_TYPE_AUTHOR":
			author = m.Get("module_author.name").String()
		case "MODULE_TYPE_CONTENT":
			for _, p := range m.Get("module_content.paragraphs").Array() {
				if len([]rune(body)) < 300 {
					for _, n := range p.Get("text.nodes").Array() {
						body += n.Get("word.words").String()
						body += n.Get("rich.text").String()
					}
					body += "\n"
				}
				for _, pic := range p.Get("pic.pics").Array() {
					if len(images) < 4 {
						images = append(images, pic.Get("url").String())
					}
				}
			}
		}
	}
	if author == "" {
		return nil, fmt.Errorf("页面未提供可读取的作者信息")
	}
	if title == "" {
		title = "B站动态/专栏"
	}
	msg := message.Message{message.Text(title, "\n作者：", author, "\n公开内容摘要：\n", clippedText(strings.TrimSpace(body), 300), "\nhttps://www.bilibili.com/opus/", id)}
	for _, p := range images {
		msg = addPublicImage(msg, p)
	}
	return msg, nil
}
func formatPublicDynamic(item gjson.Result, id string) (message.Message, error) {
	if !item.Exists() || item.Get("modules.module_author.name").String() == "" {
		return nil, fmt.Errorf("未取得可读取的动态/专栏内容")
	}
	author := item.Get("modules.module_author.name").String()
	dynamic := item.Get("modules.module_dynamic")
	major := dynamic.Get("major")
	title, body := "", dynamic.Get("desc.text").String()
	var images []string
	switch major.Get("type").String() {
	case "MAJOR_TYPE_ARTICLE":
		title = major.Get("article.title").String()
		body = major.Get("article.desc").String()
		for _, p := range major.Get("article.covers").Array() {
			images = append(images, p.String())
		}
	case "MAJOR_TYPE_OPUS":
		title = major.Get("opus.title").String()
		if body == "" {
			body = major.Get("opus.summary.text").String()
		}
		for _, p := range major.Get("opus.pics").Array() {
			images = append(images, p.Get("url").String())
		}
	case "MAJOR_TYPE_DRAW":
		for _, p := range major.Get("draw.items").Array() {
			images = append(images, p.Get("src").String())
		}
	case "MAJOR_TYPE_ARCHIVE":
		title = major.Get("archive.title").String()
		images = append(images, major.Get("archive.cover").String())
	}
	if title == "" {
		title = "B站动态/专栏"
	}
	if body == "" {
		body = "该内容未提供公开文字摘要，请打开原文查看。"
	}
	msg := message.Message{message.Text(title, "\n作者：", author, "\n", clippedText(body, 600), "\nhttps://www.bilibili.com/opus/", id)}
	for i, raw := range images {
		if i == 4 {
			break
		}
		msg = addPublicImage(msg, raw)
	}
	return msg, nil
}

func publicLiveMessage(id, cookie string) (message.Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	data, err := publicBiliJSON(ctx, "https://api.live.bilibili.com/room/v1/Room/get_info?id="+id, cookie)
	if err != nil {
		return nil, err
	}
	if data.Get("room_id").Int() <= 0 {
		return nil, fmt.Errorf("未取得有效直播间信息")
	}
	uid := data.Get("uid").String()
	author := "UID " + uid
	profile, profileErr := publicBiliJSON(ctx, "https://api.live.bilibili.com/live_user/v1/Master/info?uid="+uid, cookie)
	if profileErr == nil && profile.Get("info.uname").String() != "" {
		author = profile.Get("info.uname").String()
	}
	state := "未知状态"
	switch data.Get("live_status").Int() {
	case 0:
		state = "未开播"
	case 1:
		state = "直播中"
	case 2:
		state = "轮播中"
	}
	msg := message.Message{message.Text(data.Get("title").String(), "\n主播：", author, "\n房间：", data.Get("room_id").String(), "\n状态：", state, "\n分区：", data.Get("area_name").String(), "\nhttps://live.bilibili.com/", data.Get("room_id").String())}
	return addPublicImage(msg, data.Get("user_cover").String()), nil
}

func publicArticleMessage(id, cookie string) (message.Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	data, err := publicBiliJSON(ctx, "https://api.bilibili.com/x/article/viewinfo?id="+id, cookie)
	if err == nil {
		var card bz.Card
		err = json.Unmarshal([]byte(data.Raw), &card)
		if err == nil && card.Title != "" {
			return card.ToArticleMessage(id), nil
		}
		if err == nil {
			err = fmt.Errorf("专栏接口没有返回有效标题")
		}
	}
	msg, pageErr := publicOpusURL(ctx, "https://www.bilibili.com/read/cv"+id, "", cookie)
	if pageErr == nil {
		return msg, nil
	}
	return nil, fmt.Errorf("%v；旧专栏页面读取失败: %w", err, pageErr)
}
