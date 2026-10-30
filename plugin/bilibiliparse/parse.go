// Package bilibiliparse bilibili卡片解析
package bilibiliparse

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	bz "github.com/FloatTech/AnimeAPI/bilibili"
	"github.com/FloatTech/ZeroBot-Plugin/plugin/bilibiliparse/internal/push"
	"github.com/FloatTech/ZeroBot-Plugin/plugin/bilibiliparse/internal/query"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const (
	enableVideoSummary   = int64(0x10)
	disableVideoSummary  = ^enableVideoSummary
	bilibiliparseReferer = "https://www.bilibili.com"
	ua                   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36"
)

var (
	limit            = ctxext.NewLimiterManager(time.Second*10, 1)
	searchVideo      = `bilibili.com\\?/video\\?/(?:av(\d+)|([bB][vV][0-9a-zA-Z]+))`
	searchDynamic    = `(t\.bilibili\.com|m\.bilibili\.com\\?/dynamic|(?:www\.)?bilibili\.com\\?/opus)\\?/(\d+)`
	searchArticle    = `bilibili.com\\?/read\\?/(?:cv|mobile\\?/)(\d+)`
	searchLiveRoom   = `live.bilibili.com\\?/(\d+)`
	searchVideoRe    = regexp.MustCompile(searchVideo)
	searchDynamicRe  = regexp.MustCompile(searchDynamic)
	searchArticleRe  = regexp.MustCompile(searchArticle)
	searchLiveRoomRe = regexp.MustCompile(searchLiveRoom)
	cfg              = bz.NewCookieConfig("data/Bilibili/config.json")
)

// 插件主体
func init() {
	en := control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "B站：资料查询、链接解析与订阅",
		Help:             unifiedHelp,
		PublicDataFolder: "Bilibili",
	})
	queryEngine, _ := en.Component("bilibili", nil)
	query.Register(queryEngine)
	pushEngine, legacyPush := en.Component("bilibilipush", nil)
	configureFeatures(en, legacyPush)
	push.Register(pushEngine, featureEnabled, formatPublicDynamic)
	en.OnRegex(`((b23|acg).tv|bili2233.cn)\\?/[0-9a-zA-Z]+`, parseEnabled).SetBlock(true).Limit(limit.LimitByGroup).
		Handle(func(ctx *zero.Ctx) {
			u := ctx.State["regex_matched"].([]string)[0]
			u = strings.ReplaceAll(u, "\\", "")
			realurl, err := bz.GetRealURL("https://" + u)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			switch {
			case searchVideoRe.MatchString(realurl):
				ctx.State["regex_matched"] = searchVideoRe.FindStringSubmatch(realurl)
				handleVideo(ctx)
			case searchDynamicRe.MatchString(realurl):
				ctx.State["regex_matched"] = searchDynamicRe.FindStringSubmatch(realurl)
				handleDynamic(ctx)
			case searchArticleRe.MatchString(realurl):
				ctx.State["regex_matched"] = searchArticleRe.FindStringSubmatch(realurl)
				handleArticle(ctx)
			case searchLiveRoomRe.MatchString(realurl):
				ctx.State["regex_matched"] = searchLiveRoomRe.FindStringSubmatch(realurl)
				handleLive(ctx)
			}
		})
	en.OnRegex(`^(开启|打开|启用|关闭|关掉|禁用)视频总结$`, zero.AdminPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			if gid <= 0 {
				// 个人用户设为负数
				gid = -ctx.Event.UserID
			}
			option := ctx.State["regex_matched"].([]string)[1]
			c, ok := ctx.State["manager"].(*ctrl.Control[*zero.Ctx])
			if !ok {
				ctx.SendChain(message.Text("找不到服务!"))
				return
			}
			data := c.GetData(gid)
			switch option {
			case "开启", "打开", "启用":
				data |= enableVideoSummary
			case "关闭", "关掉", "禁用":
				data &= disableVideoSummary
			default:
				return
			}
			err := c.SetData(gid, data)
			if err != nil {
				ctx.SendChain(message.Text("出错啦: ", err))
				return
			}
			ctx.SendChain(message.Text("已", option, "视频总结"))
		})
	en.OnRegex(searchVideo, parseEnabled).SetBlock(true).Limit(limit.LimitByGroup).Handle(handleVideo)
	en.OnRegex(searchDynamic, parseEnabled).SetBlock(true).Limit(limit.LimitByGroup).Handle(handleDynamic)
	en.OnRegex(searchArticle, parseEnabled).SetBlock(true).Limit(limit.LimitByGroup).Handle(handleArticle)
	en.OnRegex(searchLiveRoom, parseEnabled).SetBlock(true).Limit(limit.LimitByGroup).Handle(handleLive)
}

func handleVideo(ctx *zero.Ctx) {
	id := ctx.State["regex_matched"].([]string)[1]
	if id == "" {
		id = ctx.State["regex_matched"].([]string)[2]
	}
	card, err := bz.GetVideoInfo(id)
	if err != nil {
		ctx.SendChain(message.Text("ERROR: ", err))
		return
	}
	msg, err := card.ToVideoMessage()
	if err != nil {
		ctx.SendChain(message.Text("ERROR: ", err))
		return
	}
	c, ok := ctx.State["manager"].(*ctrl.Control[*zero.Ctx])
	var summarize func() ([]message.Segment, error)
	if ok && c.GetData(scope(ctx))&enableVideoSummary == enableVideoSummary {
		summarize = func() ([]message.Segment, error) { return getVideoSummary(cfg, card) }
	}
	sendVideoResult(msg, fmt.Sprint("https://www.bilibili.com/video/", card.BvID), summarize, func(m []message.Segment) bool {
		return ctx.SendChain(m...).ID() != 0
	})
}

func handleDynamic(ctx *zero.Ctx) {
	cookie, err := optionalBiliCookie(cfg.Load)
	if err != nil {
		ctx.SendChain(message.Text("ERROR: 读取B站配置失败: ", err))
		return
	}
	msg, err := publicDynamicMessage(ctx.State["regex_matched"].([]string)[2], cookie)
	if err != nil {
		ctx.SendChain(message.Text("ERROR: ", err))
		return
	}
	ctx.SendChain(msg...)
}

func handleArticle(ctx *zero.Ctx) {
	cookie, err := optionalBiliCookie(cfg.Load)
	if err != nil {
		ctx.SendChain(message.Text("ERROR: 读取B站配置失败: ", err))
		return
	}
	msg, err := publicArticleMessage(ctx.State["regex_matched"].([]string)[1], cookie)
	if err != nil {
		ctx.SendChain(message.Text("ERROR: ", err))
		return
	}
	ctx.SendChain(msg...)
}

func handleLive(ctx *zero.Ctx) {
	cookie, err := optionalBiliCookie(cfg.Load)
	if err != nil {
		ctx.SendChain(message.Text("ERROR: 读取B站配置失败: ", err))
		return
	}
	msg, err := publicLiveMessage(ctx.State["regex_matched"].([]string)[1], cookie)
	if err != nil {
		ctx.SendChain(message.Text("ERROR: ", err))
		return
	}
	ctx.SendChain(msg...)
}

// getVideoSummary AI视频总结
func getVideoSummary(cookiecfg *bz.CookieConfig, card bz.Card) (msg []message.Segment, err error) {
	cookie := ""
	if cookiecfg != nil {
		cookie, err = cookiecfg.Load()
		if err != nil {
			return nil, fmt.Errorf("视频总结Cookie不可读取，请主人检查配置")
		}
	}
	return requestVideoSummary(videoSummaryClient, bz.SignURL(fmt.Sprintf(bz.VideoSummaryURL, card.BvID, card.CID, card.Owner.Mid)), cookie)
}

func decodeVideoSummary(data []byte) (msg []message.Segment, err error) {
	var videoSummary bz.VideoSummary
	var status struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &status) != nil || status.Code == nil {
		return nil, fmt.Errorf("B站视频总结接口：响应格式无效")
	}
	if *status.Code != 0 {
		return nil, fmt.Errorf("B站视频总结接口错误（%d）", *status.Code)
	}
	if len(status.Data) == 0 || string(status.Data) == "null" || json.Unmarshal(data, &videoSummary) != nil {
		return nil, fmt.Errorf("B站视频总结接口：缺少有效数据")
	}
	if videoSummary.Data.Code != 0 {
		return nil, fmt.Errorf("B站视频总结不可用（业务码%d）", videoSummary.Data.Code)
	}

	msg = make([]message.Segment, 0, 16)
	if videoSummary.Data.ModelResult.Summary == `` {
		msg = append(msg, message.Text("该视频暂无可用的B站视频总结。"))
		return
	}
	msg = append(msg, message.Text("B站提供的视频总结\n\n"))
	msg = append(msg, message.Text(videoSummary.Data.ModelResult.Summary, "\n\n"))
	for _, v := range videoSummary.Data.ModelResult.Outline {
		msg = append(msg, message.Text("● ", v.Title, "\n"))
		for _, p := range v.PartOutline {
			msg = append(msg, message.Text(fmt.Sprintf("%d:%d %s\n", p.Timestamp/60, p.Timestamp%60, p.Content)))
		}
		msg = append(msg, message.Text("\n"))
	}
	return
}
