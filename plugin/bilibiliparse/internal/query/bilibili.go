// Package bilibili 查询b站用户信息
package query

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/FloatTech/ZeroBot-Plugin/plugin/bilibiliparse/internal/userselect"
	"image"
	"image/color"
	"os"
	"path"
	"sort"
	"strconv"
	"time"

	bz "github.com/FloatTech/AnimeAPI/bilibili"
	fcext "github.com/FloatTech/floatbox/ctxext"
	"github.com/FloatTech/floatbox/file"
	"github.com/FloatTech/floatbox/web"
	"github.com/FloatTech/gg"
	"github.com/FloatTech/gg/factory"
	"github.com/FloatTech/gg/fio"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/img/text"
	log "github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var (
	danmakuTypeMap = map[int64]string{
		0: "普通消息",
		1: "礼物",
		2: "上舰",
		3: "Superchat",
		4: "进入直播间",
		5: "标题变动",
		6: "分区变动",
		7: "直播中止",
		8: "直播继续",
	}
	cfg = bz.NewCookieConfig("data/Bilibili/config.json")
)

// 查成分的
func Register(engine *control.Engine) {
	cachePath := engine.DataFolder() + "cache/"
	_ = os.RemoveAll(cachePath)
	_ = os.MkdirAll(cachePath, 0755)
	var getdb = fcext.DoOnceOnSuccess(func(ctx *zero.Ctx) bool {
		var err error
		_, _ = engine.GetLazyData("bilibili.db", false)
		vdb, err = initializeVup(engine.DataFolder() + "bilibili.db")
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return false
		}
		return true
	})
	engine.OnRegex(`^>user info\s?(.{1,25})$`, userselect.RequireUser(cfg)).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			id := ctx.State["uid"].(string)
			card, err := getMemberCard(id)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text(
				"uid: ", card.Mid, "\n",
				"name: ", card.Name, "\n",
				"sex: ", card.Sex, "\n",
				"sign: ", card.Sign, "\n",
				"level: ", card.LevelInfo.CurrentLevel, "\n",
				"birthday: ", card.Birthday,
			))
		})

	engine.OnRegex(`^>vup info\s?(.{1,25})$`, userselect.RequireUser(cfg)).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			id := ctx.State["uid"].(string)
			// 获取详情
			fo, err := bz.GetVtbDetail(id)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text(
				"b站id: ", fo.Mid, "\n",
				"名字: ", fo.Uname, "\n",
				"当前粉丝数: ", fo.Follower, "\n",
				"24h涨粉数: ", fo.Rise, "\n",
				"视频投稿数: ", fo.Video, "\n",
				"直播间id: ", fo.Roomid, "\n",
				"舰队: ", fo.GuardNum, "\n",
				"直播总排名: ", fo.AreaRank, "\n",
				"数据来源: ", "https://vtbs.moe/detail/", fo.Mid, "\n",
				"数据获取时间: ", time.Now().Format("2006-01-02 15:04:05"),
			))
		})

	engine.OnRegex(`^查成分\s?(.{1,25})$`, userselect.RequireUser(cfg), getdb).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			id := ctx.State["uid"].(string)
			today := time.Now().Format("20060102")
			drawedFile := cachePath + id + today + "vupLike.png"
			if file.IsExist(drawedFile) {
				ctx.SendChain(message.Image("file:///" + file.BOTPATH + "/" + drawedFile))
				return
			}
			u, medals, profileErr, medalErr := compositionSources(id, getMemberCard, cfg.GetMedalWall)
			if profileErr != nil && medalErr != nil {
				ctx.SendChain(message.Text("ERROR: 用户资料和粉丝牌均未能获取，请稍后重试。", profileErr, "；", medalErr))
				return
			}
			if profileErr != nil {
				u = bz.MemberCard{Mid: id, Name: "UID " + id}
				// Partial results must never satisfy the daily complete-result cache.
				drawedFile = cachePath + id + strconv.FormatInt(time.Now().UnixNano(), 10) + "partial.png"
			}
			vups, err := vdb.filterVup(u.Attentions)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			vupLen := len(vups)
			sort.Sort(bz.MedalSorter(medals))
			if medalErr != nil {
				ctx.SendChain(message.Text("粉丝牌暂不可用，本次仅展示可读取的关注信息。"))
				drawedFile = cachePath + id + strconv.FormatInt(time.Now().UnixNano(), 10) + "partial.png"
			}
			frontVups := make([]vup, 0)
			medalMap := make(map[int64]bz.Medal)
			for _, v := range medals {
				up := vup{
					Mid:   v.Mid,
					Uname: v.Uname,
				}
				frontVups = append(frontVups, up)
				medalMap[v.Mid] = v
			}
			vups = append(vups, frontVups...)
			copy(vups[len(frontVups):], vups)
			copy(vups, frontVups)
			for i := len(frontVups); i < len(vups); i++ {
				if _, ok := medalMap[vups[i].Mid]; ok {
					vups = append(vups[:i], vups[i+1:]...)
					i--
				}
			}
			facePath := cachePath + id + "vupFace" + path.Ext(u.Face)
			backX := 500
			backY := 500
			var back image.Image
			if u.Face != "" && path.Ext(u.Face) != ".webp" {
				err = initFacePic(facePath, u.Face)
				if err != nil {
					ctx.SendChain(message.Text("ERROR: ", err))
					return
				}
				back, err = fio.LoadImage(facePath)
				if err != nil {
					ctx.SendChain(message.Text("ERROR: ", err))
					return
				}
				back = factory.Size(back, backX, backY).Image()
			}
			if len(vups) > 50 {
				ctx.SendChain(message.Text(u.Name + "关注的up主太多了, 只展示前50个up"))
				vups = vups[:50]
			}
			canvas := gg.NewContext(1500, int(500*(1.1+float64(len(vups))/3)))
			fontSize := 50.0
			canvas.SetColor(color.White)
			canvas.Clear()
			if back != nil {
				canvas.DrawImage(back, 0, 0)
			}
			canvas.SetColor(color.Black)
			data, err := file.GetLazyData(text.BoldFontFile, control.Md5File, true)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
			}
			if err = canvas.ParseFontFace(data, fontSize); err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			sl, _ := canvas.MeasureString("好")
			length, h := canvas.MeasureString(u.Mid)
			n, _ := canvas.MeasureString(u.Name)
			canvas.DrawString(u.Name, 550, 160-h)
			canvas.DrawRoundedRectangle(600+n-length*0.1, 160-h*2.5, length*1.2, h*2, fontSize*0.2)
			canvas.SetRGB255(221, 221, 221)
			canvas.Fill()
			canvas.SetColor(color.Black)
			canvas.DrawString(u.Mid, 600+n, 160-h)
			for i, line := range compositionStats(u, vupLen, profileErr != nil) {
				canvas.DrawString(line, 550, float64(240+80*i)-h)
			}
			canvas.DrawString("查询日期："+time.Now().Format("2006-01-02"), 550, 480-h)
			for i, v := range vups {
				if i%2 == 1 {
					canvas.SetRGB255(245, 245, 245)
					canvas.DrawRectangle(0, float64(backY)*1.1+float64(i)*float64(backY)/3, float64(backX*3), float64(backY)/3)
					canvas.Fill()
				}
				canvas.SetColor(color.Black)
				nl, _ := canvas.MeasureString(v.Uname)
				canvas.DrawString(v.Uname, float64(backX)*0.1, float64(backY)*1.1+float64(i+1)*float64(backY)/3-2*h)
				ml, _ := canvas.MeasureString(strconv.FormatInt(v.Mid, 10))
				canvas.DrawRoundedRectangle(nl-0.1*ml+float64(backX)*0.2, float64(backY)*1.1+float64(i+1)*float64(backY)/3-h*3.5, ml*1.2, h*2, fontSize*0.2)
				canvas.SetRGB255(221, 221, 221)
				canvas.Fill()
				canvas.SetColor(color.Black)
				canvas.DrawString(strconv.FormatInt(v.Mid, 10), nl+float64(backX)*0.2, float64(backY)*1.1+float64(i+1)*float64(backY)/3-2*h)
				if m, ok := medalMap[v.Mid]; ok {
					mnl, _ := canvas.MeasureString(m.MedalName)
					grad := gg.NewLinearGradient(nl+ml-sl/2+float64(backX)*0.4, float64(backY)*1.1+float64(i+1)*float64(backY)/3-3.5*h, nl+ml+mnl+sl/2+float64(backX)*0.4, float64(backY)*1.1+float64(i+1)*float64(backY)/3-1.5*h)
					r, g, b := int2rbg(m.MedalColorStart)
					grad.AddColorStop(0, color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255})
					r, g, b = int2rbg(m.MedalColorEnd)
					grad.AddColorStop(1, color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255})
					canvas.SetFillStyle(grad)
					canvas.SetLineWidth(4)
					canvas.MoveTo(nl+ml-sl/2+float64(backX)*0.4, float64(backY)*1.1+float64(i+1)*float64(backY)/3-3.5*h)
					canvas.LineTo(nl+ml+mnl+sl/2+float64(backX)*0.4, float64(backY)*1.1+float64(i+1)*float64(backY)/3-3.5*h)
					canvas.LineTo(nl+ml+mnl+sl/2+float64(backX)*0.4, float64(backY)*1.1+float64(i+1)*float64(backY)/3-1.5*h)
					canvas.LineTo(nl+ml-sl/2+float64(backX)*0.4, float64(backY)*1.1+float64(i+1)*float64(backY)/3-1.5*h)
					canvas.ClosePath()
					canvas.Fill()
					canvas.SetColor(color.White)
					canvas.DrawString(m.MedalName, nl+ml+float64(backX)*0.4, float64(backY)*1.1+float64(i+1)*float64(backY)/3-2*h)
					r, g, b = int2rbg(m.MedalColorBorder)
					canvas.SetRGB255(int(r), int(g), int(b))
					canvas.DrawString(strconv.FormatInt(m.Level, 10), nl+ml+mnl+sl+float64(backX)*0.4, float64(backY)*1.1+float64(i+1)*float64(backY)/3-2*h)
					mll, _ := canvas.MeasureString(strconv.FormatInt(m.Level, 10))
					canvas.SetLineWidth(4)
					canvas.MoveTo(nl+ml-sl/2+float64(backX)*0.4, float64(backY)*1.1+float64(i+1)*float64(backY)/3-3.5*h)
					canvas.LineTo(nl+ml+mnl+mll+sl/2+float64(backX)*0.5, float64(backY)*1.1+float64(i+1)*float64(backY)/3-3.5*h)
					canvas.LineTo(nl+ml+mnl+mll+sl/2+float64(backX)*0.5, float64(backY)*1.1+float64(i+1)*float64(backY)/3-1.5*h)
					canvas.LineTo(nl+ml-sl/2+float64(backX)*0.4, float64(backY)*1.1+float64(i+1)*float64(backY)/3-1.5*h)
					canvas.ClosePath()
					canvas.Stroke()
				}
			}
			f, err := os.Create(drawedFile)
			if err != nil {
				log.Errorln("[bilibili]", err)
				data, err := factory.ToBytes(canvas.Image())
				if err != nil {
					log.Errorln("[bilibili]", err)
					return
				}
				ctx.SendChain(message.ImageBytes(data))
				return
			}
			_, err = factory.WriteTo(canvas.Image(), f)
			_ = f.Close()
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Image("file:///" + file.BOTPATH + "/" + drawedFile))
		})

	engine.OnRegex(`^查弹幕\s?(\S{1,25})\s?(\d*)$`, userselect.RequireUser(cfg)).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		id := ctx.State["uid"].(string)
		pagenum := ctx.State["regex_matched"].([]string)[2]
		if pagenum == "" {
			pagenum = "0"
		}
		requestCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		client := newDanmakuClient()
		defer client.CloseIdleConnections()
		danmaku, err := getDanmaku(requestCtx, client, fmt.Sprintf(bz.DanmakuAPI, id, pagenum))
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		if len(danmaku.Data.Data.Records) == 0 {
			ctx.SendChain(message.Text("弹幕记录接口未收录该用户在此页的记录（UID：", id, "）。"))
			return
		}
		u, profileErr := memberCard(requestCtx, client, fmt.Sprintf(bz.MemberCardURL, id))
		if profileErr != nil {
			u = bz.MemberCard{Mid: id, Name: "UID " + id}
		}
		today := time.Now().Format("20060102150415")
		drawedFile := cachePath + id + today + "vupLike.png"
		avatarClient := web.NewDefaultClient()
		defer avatarClient.CloseIdleConnections()
		addresses := []string{u.Face}
		for _, record := range danmaku.Data.Data.Records {
			addresses = append(addresses, record.Channel.FaceURL)
		}
		avatars := danmakuAvatars(requestCtx, avatarClient, addresses)
		data, err := file.GetLazyData(text.BoldFontFile, control.Md5File, true)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		nim, err := renderDanmakuReport(u, danmaku, profileErr != nil, avatars, data)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		f, err := os.Create(drawedFile)
		if err != nil {
			log.Errorln("[bilibili]", err)
			data, err := factory.ToBytes(nim)
			if err != nil {
				log.Errorln("[bilibili]", err)
				return
			}
			ctx.SendChain(message.ImageBytes(data))
			return
		}
		_, err = factory.WriteTo(nim, f)
		_ = f.Close()
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.SendChain(message.Image("file:///" + file.BOTPATH + "/" + drawedFile))
	})

	engine.OnRegex(`^设置b站cookie?\s+(.*)$`, zero.SuperUserPermission, zero.OnlyPrivate).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			cookie := ctx.State["regex_matched"].([]string)[1]
			err := cfg.Set(cookie)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("B站Cookie已保存。"))
		})

	engine.OnFullMatch("更新vup", zero.SuperUserPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			ctx.SendChain(message.Text("少女祈祷中..."))
			err := updateVup(engine.DataFolder() + "bilibili.db")
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("vup已更新"))
		})
}

func initFacePic(filename, faceURL string) error {
	if file.IsNotExist(filename) {
		data, err := web.GetData(faceURL)
		if err != nil {
			return err
		}
		err = os.WriteFile(filename, data, 0666)
		if err != nil {
			return err
		}
	}
	return nil
}

func int2rbg(t int64) (int64, int64, int64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(t))
	b, g, r := int64(buf[0]), int64(buf[1]), int64(buf[2])
	return r, g, b
}
