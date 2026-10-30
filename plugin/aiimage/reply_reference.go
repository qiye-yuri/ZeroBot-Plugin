package aiimage

import (
	"fmt"
	"github.com/tidwall/gjson"
	"github.com/wdvxdr1123/ZeroBot/message"
	"strconv"
	"strings"
)

func imageCommand(msg message.Message) (string, bool) {
	text := strings.TrimSpace(msg.ExtractPlainText())
	if !strings.HasPrefix(text, "AI画图") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(text, "AI画图")), true
}
func referenceSources(msg message.Message, bot, user, group int64, allowed func(string) (bool, error), fetch func(int64) (gjson.Result, error), cutoff int64) ([]referenceSource, error) {
	sources := []referenceSource{}
	reply := ""
	for _, seg := range msg {
		switch seg.Type {
		case "image":
			sources = append(sources, referenceSource{URL: seg.Data["url"], File: seg.Data["file"]})
		case "reply":
			if reply != "" {
				return nil, fmt.Errorf("每次只支持引用一条消息")
			}
			reply = seg.Data["id"]
			if reply == "" {
				return nil, fmt.Errorf("引用消息编号无效")
			}
		}
	}
	// Current attachments come first; validated quoted images are appended.
	if reply == "" {
		return sources, nil
	}
	id, err := strconv.ParseInt(reply, 10, 64)
	if err != nil || id == 0 {
		return nil, fmt.Errorf("引用消息编号无效")
	}
	ok, err := allowed(strconv.FormatInt(id, 10))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("引用消息不在允许的测试/会话范围，未读取图片")
	}
	data, err := fetch(id)
	if err != nil {
		return nil, fmt.Errorf("引用消息读取失败，可能已撤回或过期，请重新附上原图")
	}
	if data.Get("message_id").Int() != id {
		return nil, fmt.Errorf("引用消息回执编号不一致")
	}
	if self := data.Get("self_id").Int(); self != 0 && self != bot {
		return nil, fmt.Errorf("引用消息不属于当前机器人")
	}
	if group != 0 {
		if data.Get("group_id").Int() != group {
			return nil, fmt.Errorf("引用图片不属于当前群聊")
		}
	} else {
		if data.Get("group_id").Int() != 0 || data.Get("message_type").String() == "group" {
			return nil, fmt.Errorf("不能从私聊读取群消息图片")
		}
		sender := data.Get("sender.user_id").Int()
		peer := data.Get("user_id").Int()
		target := data.Get("target_id").Int()
		incoming := sender == user && (target == 0 || target == bot) && (peer == 0 || peer == user)
		outgoing := sender == bot && (target == user || peer == user)
		if !incoming && !outgoing {
			return nil, fmt.Errorf("无法确认引用图片属于当前私聊，请重新附上原图")
		}
	}
	if cutoff > 0 && (data.Get("time").Int() <= cutoff) {
		return nil, fmt.Errorf("引用消息属于历史测试边界或时间未知，请重新附上原图")
	}
	raw := data.Get("message")
	if !raw.Exists() {
		raw = data.Get("raw_message")
	}
	var segments message.Message
	if raw.IsArray() {
		segments = message.ParseMessage([]byte(raw.Raw))
	} else {
		segments = message.ParseMessageFromString(raw.String())
	}
	for _, seg := range segments {
		if seg.Type == "image" {
			sources = append(sources, referenceSource{URL: seg.Data["url"], File: seg.Data["file"]})
		}
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("被引用的消息中没有可用图片，请回复图片消息或直接附上原图")
	}
	return sources, nil
}
