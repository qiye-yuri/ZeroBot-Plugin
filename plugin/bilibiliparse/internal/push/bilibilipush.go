// Package bilibilipush b站推送
package push

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/FloatTech/ZeroBot-Plugin/plugin/bilibiliparse/internal/userselect"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	bz "github.com/FloatTech/AnimeAPI/bilibili"
	"github.com/FloatTech/floatbox/binary"
	"github.com/FloatTech/floatbox/web"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/img/text"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const (
	ua      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36"
	referer = "https://space.bilibili.com/%v"
	infoURL = "https://api.bilibili.com/x/space/wbi/acc/info?mid=%v"
)

// bdb bilibili推送数据库
var bdb *bilibilipushdb
var canPush func(int64, string) bool
var dynamicMessage func(gjson.Result, string) (message.Message, error)
var pushMu sync.Mutex

var (
	lastTime   = map[int64]int64{}
	liveStatus = map[int64]int{}
	upMap      = map[int64]string{}
	cfg        = bz.NewCookieConfig("data/Bilibili/config.json")
)

func Register(en *control.Engine, enabled func(int64, string) bool, formatter func(gjson.Result, string) (message.Message, error)) {
	canPush = enabled
	dynamicMessage = formatter

	// 加载bilibili推送数据库
	dbpath := "data/bilibilipush/"
	if err := os.MkdirAll(dbpath, 0755); err != nil {
		panic(err)
	}
	dbfile := dbpath + "push.db"
	bdb = initializePush(dbfile)
	en.OnFullMatch(`开启艾特全体`, zero.UserOrGrpAdmin, zero.OnlyGroup).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		gid := ctx.Event.GroupID
		if err := changeAtAll(gid, 1); err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.SendChain(message.Text("已开启艾特全体Oo"))
	})

	en.OnFullMatch(`关闭艾特全体`, zero.UserOrGrpAdmin, zero.OnlyGroup).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		gid := ctx.Event.GroupID
		if err := changeAtAll(gid, 0); err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.SendChain(message.Text("已关闭艾特全体Oo"))
	})

	en.OnRegex(`^添加[Bb]站订阅\s?(.{1,25})$`, zero.UserOrGrpAdmin, userselect.RequireUser(cfg)).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		buid, _ := strconv.ParseInt(ctx.State["uid"].(string), 10, 64)
		name, err := getName(buid, cfg)
		if err != nil || name == "" {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		gid := ctx.Event.GroupID
		if gid == 0 {
			gid = -ctx.Event.UserID
		}
		if err := subscribe(buid, gid); err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.SendChain(message.Text("已添加" + name + "的订阅"))
	})

	en.OnRegex(`^取消[Bb]站订阅\s?(.{1,25})$`, zero.UserOrGrpAdmin, userselect.RequireUser(cfg)).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		buid, _ := strconv.ParseInt(ctx.State["uid"].(string), 10, 64)
		name, err := getName(buid, cfg)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		gid := ctx.Event.GroupID
		if gid == 0 {
			gid = -ctx.Event.UserID
		}
		if err := unsubscribe(buid, gid); err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.SendChain(message.Text("已取消" + name + "的订阅"))
	})
	en.OnRegex(`^取消[Bb]站动态订阅\s?(.{1,25})$`, zero.UserOrGrpAdmin, userselect.RequireUser(cfg)).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		buid, _ := strconv.ParseInt(ctx.State["uid"].(string), 10, 64)
		name, err := getName(buid, cfg)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		gid := ctx.Event.GroupID
		if gid == 0 {
			gid = -ctx.Event.UserID
		}
		if err := unsubscribeDynamic(buid, gid); err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.SendChain(message.Text("已取消" + name + "的动态订阅"))
	})
	en.OnRegex(`^取消[Bb]站直播订阅\s?(.{1,25})$`, zero.UserOrGrpAdmin, userselect.RequireUser(cfg)).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		buid, _ := strconv.ParseInt(ctx.State["uid"].(string), 10, 64)
		gid := ctx.Event.GroupID
		if gid == 0 {
			gid = -ctx.Event.UserID
		}
		name, err := getName(buid, cfg)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		if err := unsubscribeLive(buid, gid); err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.SendChain(message.Text("已取消" + name + "的直播订阅"))
	})

	en.OnRegex(`^[Bb]站推送列表$`, zero.UserOrGrpAdmin).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		gid := ctx.Event.GroupID
		if gid == 0 {
			gid = -ctx.Event.UserID
		}
		bpl := bdb.getAllPushByGroup(gid)
		msg := "--------B站推送列表--------"
		for _, v := range bpl {
			if _, ok := upMap[v.BilibiliUID]; !ok {
				bdb.updateAllUp()
			}
			msg += fmt.Sprintf("\nuid:%-12d 动态：", v.BilibiliUID)
			if v.DynamicDisable == 0 {
				msg += "●"
			} else {
				msg += "○"
			}
			msg += " 直播："
			if v.LiveDisable == 0 {
				msg += "●"
			} else {
				msg += "○"
			}
			msg += " up主：" + upMap[v.BilibiliUID]
		}
		data, err := text.RenderToBase64(msg, text.FontFile, 600, 20)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		if id := ctx.SendChain(message.Image("base64://" + binary.BytesToString(data))); id.ID() == 0 {
			ctx.SendChain(message.Text("ERROR: 可能被风控了"))
		}
	})
	en.OnRegex(`^拉取[Bb]站推送$`, zero.SuperUserPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		if !pushMu.TryLock() {
			ctx.SendChain(message.Text("B站推送正在拉取，请稍后。"))
			return
		}
		defer pushMu.Unlock()
		err := sendDynamic(ctx)
		if err != nil {
			ctx.SendPrivateMessage(ctx.Event.UserID, message.Message{message.Text("Error: bilibilipush,", err)})
		}
		err = sendLive(ctx)
		if err != nil {
			ctx.SendPrivateMessage(ctx.Event.UserID, message.Message{message.Text("Error: bilibilipush,", err)})
		}
	})
}

func changeAtAll(gid int64, b int) (err error) {
	bpMap := map[string]any{
		"group_id": gid,
		"at_all":   b,
	}
	return bdb.updateAtAll(bpMap)
}

// 取得uid的名字
func getName(buid int64, cookiecfg *bz.CookieConfig) (name string, err error) {
	var ok bool
	if name, ok = upMap[buid]; !ok {
		data, err := web.RequestDataWithHeaders(web.NewDefaultClient(), bz.SignURL(fmt.Sprintf(infoURL, buid)), "GET", func(r *http.Request) error {
			if cookiecfg != nil {
				cookie := ""
				cookie, err = cookiecfg.Load()
				if err != nil {
					return err
				}
				r.Header.Add("Cookie", cookie)
			}
			r.Header.Set("User-Agent", ua)
			return nil
		}, nil)
		if err != nil {
			return "", err
		}
		status := int(gjson.Get(binary.BytesToString(data), "code").Int())
		if status != 0 {
			err = errors.New(gjson.Get(binary.BytesToString(data), "message").String())
			return "", err
		}
		name = gjson.Get(binary.BytesToString(data), "data.name").String()
		bdb.insertBilibiliUp(buid, name)
		upMap[buid] = name
	}
	return
}

// subscribe 订阅
func subscribe(buid, groupid int64) (err error) {
	bpMap := map[string]any{
		"bilibili_uid":    buid,
		"group_id":        groupid,
		"live_disable":    0,
		"dynamic_disable": 0,
	}
	return bdb.insertOrUpdateLiveAndDynamic(bpMap)
}

// unsubscribe 取消订阅
func unsubscribe(buid, groupid int64) (err error) {
	bpMap := map[string]any{
		"bilibili_uid":    buid,
		"group_id":        groupid,
		"live_disable":    1,
		"dynamic_disable": 1,
	}
	return bdb.insertOrUpdateLiveAndDynamic(bpMap)
}

func unsubscribeDynamic(buid, groupid int64) (err error) {
	bpMap := map[string]any{
		"bilibili_uid":    buid,
		"group_id":        groupid,
		"dynamic_disable": 1,
	}
	return bdb.insertOrUpdateLiveAndDynamic(bpMap)
}

func unsubscribeLive(buid, groupid int64) (err error) {
	bpMap := map[string]any{
		"bilibili_uid": buid,
		"group_id":     groupid,
		"live_disable": 1,
	}
	return bdb.insertOrUpdateLiveAndDynamic(bpMap)
}

func getUserDynamicCard(buid int64, cookiecfg *bz.CookieConfig) ([]gjson.Result, error) {
	cookie, err := cookiecfg.Load()
	if err != nil {
		return nil, err
	}
	return fetchDynamicFeed(newFeedClient(), fmt.Sprintf("https://api.bilibili.com/x/polymer/web-dynamic/v1/feed/space?host_mid=%d", buid), cookie)
}

func getLiveList(uids ...int64) (string, error) {
	m := make(map[string]any)
	m["uids"] = uids
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	data, err := web.PostData(bz.LiveListURL, "application/json", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	return binary.BytesToString(data), nil
}

func sendDynamic(ctx *zero.Ctx) error {
	uids := bdb.getAllBuidByDynamic()
	for _, buid := range uids {
		time.Sleep(2 * time.Second)
		cardList, err := getUserDynamicCard(buid, cfg)
		if err != nil {
			return err
		}
		if len(cardList) == 0 {
			continue
		}
		t, initialized := lastTime[buid]
		pending, newest := pendingDynamics(cardList, t, initialized, time.Now().Unix())
		for _, item := range pending {
			if canPush == nil {
				continue
			}
			msg, err := dynamicMessage(item, item.Get("id_str").String())
			if err != nil {
				return err
			}
			for _, gid := range bdb.getAllGroupByBuidAndDynamic(buid) {
				if !canPush(gid, "动态推送") {
					continue
				}
				time.Sleep(time.Millisecond * 100)
				if gid > 0 {
					ctx.SendGroupMessage(gid, msg)
				} else if gid < 0 {
					ctx.SendPrivateMessage(-gid, msg)
				}
			}
		}
		lastTime[buid] = newest
	}
	return nil
}

func sendLive(ctx *zero.Ctx) error {
	uids := bdb.getAllBuidByLive()
	if len(uids) == 0 {
		return nil
	}
	ll, err := getLiveList(uids...)
	if err != nil {
		return err
	}
	gjson.Get(ll, "data").ForEach(func(key, value gjson.Result) bool {
		newStatus := int(value.Get("live_status").Int())
		if newStatus == 2 {
			newStatus = 0
		}
		if _, ok := liveStatus[key.Int()]; !ok {
			liveStatus[key.Int()] = newStatus
			return true
		}
		oldStatus := liveStatus[key.Int()]
		if newStatus != oldStatus && newStatus == 1 {
			liveStatus[key.Int()] = newStatus
			if canPush != nil {
				groupList := bdb.getAllGroupByBuidAndLive(key.Int())
				roomID := value.Get("short_id").Int()
				if roomID == 0 {
					roomID = value.Get("room_id").Int()
				}
				lURL := bz.LiveURL + strconv.FormatInt(roomID, 10)
				lName := value.Get("uname").String()
				lTitle := value.Get("title").String()
				lCover := value.Get("cover_from_user").String()
				if lCover == "" {
					lCover = value.Get("keyframe").String()
				}
				var msg []message.Segment
				msg = append(msg, message.Text(lName+" 正在直播：\n"))
				msg = append(msg, message.Text(lTitle))
				msg = append(msg, message.Image(lCover))
				msg = append(msg, message.Text("直播链接：", lURL))
				for _, gid := range groupList {
					if canPush(gid, "直播推送") {
						time.Sleep(time.Millisecond * 100)
						targetMsg := append(message.Message(nil), msg...)
						switch {
						case gid > 0:
							if res := bdb.getAtAll(gid); res == 1 {
								targetMsg = append([]message.Segment{message.AtAll()}, targetMsg...)
							}
							ctx.SendGroupMessage(gid, targetMsg)
						case gid < 0:
							ctx.SendPrivateMessage(-gid, targetMsg)
						}
					}
				}
			}
		} else if newStatus != oldStatus {
			liveStatus[key.Int()] = newStatus
		}
		return true
	})
	return nil
}
