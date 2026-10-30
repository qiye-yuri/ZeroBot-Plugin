package query

import (
	"fmt"
	bz "github.com/FloatTech/AnimeAPI/bilibili"
	"github.com/FloatTech/gg"
	"image"
	"image/color"
	"strconv"
	"time"
)

// Render the collected records independently of network and QQ delivery.
func renderDanmakuReport(u bz.MemberCard, danmaku bz.Danmakusuki, profileUnavailable bool, avatars []image.Image, data []byte) (image.Image, error) {
	var err error
	canvas := gg.NewContext(100, 100)
	fontSize := 50.0
	if err = canvas.ParseFontFace(data, fontSize); err != nil {
		return nil, err
	}
	dz, h := canvas.MeasureString("好")
	danmuH := h * 2
	faceH := float64(510)

	totalDanmuku := 0
	for i := 0; i < len(danmaku.Data.Data.Records); i++ {
		totalDanmuku += len(danmaku.Data.Data.Records[i].Danmakus) + 1
	}
	cw := 3000
	mcw := float64(2000)
	ch := 550 + len(danmaku.Data.Data.Records)*int(faceH) + totalDanmuku*int(danmuH)
	canvas = gg.NewContext(cw, ch)
	canvas.SetColor(color.White)
	canvas.Clear()
	canvas.SetColor(color.Black)
	if err = canvas.ParseFontFace(data, fontSize); err != nil {
		return nil, err
	}
	facestart := 100
	fontH := h * 1.6
	startWidth := float64(700)
	startWidth2 := float64(20)

	drawDanmakuAvatar(canvas, avatars[0], facestart, 0)
	canvas.SetColor(color.Black)
	length, _ := canvas.MeasureString(u.Mid)
	n, _ := canvas.MeasureString(u.Name)
	canvas.DrawString(u.Name, startWidth, 122.5)
	canvas.DrawRoundedRectangle(900+n-length*0.1, 66, length*1.2, 75, fontSize*0.2)
	canvas.SetRGB255(221, 221, 221)
	canvas.Fill()
	canvas.SetColor(color.Black)
	canvas.DrawString(u.Mid, 900+n, 122.5)
	if profileUnavailable {
		canvas.DrawString("用户资料暂不可用，以下为已收录弹幕", startWidth, 222.5)
	} else {
		canvas.DrawString(fmt.Sprintf("粉丝：%d   关注：%d", u.Fans, u.Attention), startWidth, 222.5)
	}
	canvas.DrawString(fmt.Sprintf("页码：[%d/%d]", danmaku.Data.PageNum, (danmaku.Data.Total-1)/5), startWidth, 322.5)
	canvas.DrawString("网页链接: "+fmt.Sprintf(bz.DanmakuURL, u.Mid), startWidth, 422.5)
	var channelStart float64
	channelStart = float64(550)
	for i := 0; i < len(danmaku.Data.Data.Records); i++ {
		item := danmaku.Data.Data.Records[i]
		drawDanmakuAvatar(canvas, avatars[i+1], facestart, int(channelStart))
		canvas.SetRGB255(24, 144, 255)
		canvas.DrawString("标题: "+item.Live.Title, startWidth, channelStart+fontH)
		canvas.DrawString("主播: "+item.Channel.UName, startWidth, channelStart+fontH*2)
		canvas.SetColor(color.Black)
		canvas.DrawString("开始时间: "+time.UnixMilli(item.Live.StartDate).Format("2006-01-02 15:04:05"), startWidth, channelStart+fontH*3)
		if item.Live.IsFinish {
			canvas.DrawString("结束时间: "+time.UnixMilli(item.Live.StopDate).Format("2006-01-02 15:04:05"), startWidth, channelStart+fontH*4)
			canvas.DrawString("直播时长: "+strconv.FormatFloat(float64(item.Live.StopDate-item.Live.StartDate)/3600000.0, 'f', 1, 64)+"小时", startWidth, channelStart+fontH*5)
		} else {
			t := "结束时间:"
			l, _ := canvas.MeasureString(t)
			canvas.DrawString(t, startWidth, channelStart+fontH*4)

			canvas.SetRGB255(0, 128, 0)
			t = "正在直播"
			canvas.DrawString(t, startWidth+l*1.1, channelStart+fontH*4)
			canvas.SetColor(color.Black)

			canvas.DrawString("直播时长: "+strconv.FormatFloat(float64(time.Now().UnixMilli()-item.Live.StartDate)/3600000.0, 'f', 1, 64)+"小时", startWidth, channelStart+fontH*5)
		}
		canvas.DrawString("弹幕数量: "+strconv.Itoa(item.Live.DanmakusCount), startWidth, channelStart+fontH*6)
		canvas.DrawString("观看次数: "+strconv.Itoa(item.Live.WatchCount), startWidth, channelStart+fontH*7)

		t := "收益:"
		l, _ := canvas.MeasureString(t)
		canvas.DrawString(t, startWidth, channelStart+fontH*8)

		t = "￥" + strconv.Itoa(int(item.Live.TotalIncome))
		canvas.SetRGB255(255, 0, 0)
		canvas.DrawString(t, startWidth+l*1.1, channelStart+fontH*8)
		canvas.SetColor(color.Black)

		DanmakuStart := channelStart + faceH
		for i := 0; i < len(item.Danmakus); i++ {
			moveW := startWidth2
			danmuNow := DanmakuStart + danmuH*float64(i+1)
			danItem := item.Danmakus[i]

			t := time.UnixMilli(danItem.SendDate).Format("15:04:05")
			l, _ := canvas.MeasureString(t)
			canvas.DrawString(t, moveW, danmuNow)
			moveW += l + dz

			t = danItem.UName
			l, _ = canvas.MeasureString(t)
			canvas.SetRGB255(24, 144, 255)
			canvas.DrawString(t, moveW, danmuNow)
			canvas.SetColor(color.Black)
			moveW += l + dz

			switch danItem.Type {
			case 0:
				t = danItem.Message
				l, _ = canvas.MeasureString(t)
				canvas.DrawString(t, moveW, danmuNow)
				moveW += l + dz
			case 1:
				t = danmakuTypeMap[danItem.Type]
				l, _ = canvas.MeasureString(t)
				canvas.SetRGB255(255, 0, 0)
				canvas.DrawString(t, moveW, danmuNow)
				moveW += l + dz

				t = danItem.Message
				l, _ = canvas.MeasureString(t)
				canvas.DrawString(t, moveW, danmuNow)
				canvas.SetColor(color.Black)
				moveW += l + dz
			case 2, 3:
				t = danmakuTypeMap[danItem.Type]
				l, _ = canvas.MeasureString(t)
				if danItem.Type == 3 {
					canvas.SetRGB255(0, 85, 255)
				} else {
					canvas.SetRGB255(128, 0, 128)
				}

				canvas.DrawString(t, moveW, danmuNow)
				moveW += l + dz

				t = danItem.Message
				l, _ = canvas.MeasureString(t)
				canvas.DrawString(t, moveW, danmuNow)
				moveW += l

				t = "["
				l, _ = canvas.MeasureString(t)
				canvas.DrawString(t, moveW, danmuNow)
				moveW += l

				t = "￥" + strconv.FormatFloat(danItem.Price, 'f', 1, 64)
				l, _ = canvas.MeasureString(t)
				canvas.SetRGB255(255, 0, 0)
				canvas.DrawString(t, moveW, danmuNow)
				if danItem.Type == 3 {
					canvas.SetRGB255(0, 85, 255)
				} else {
					canvas.SetRGB255(128, 0, 128)
				}
				moveW += l

				t = "]"
				l, _ = canvas.MeasureString(t)
				canvas.DrawString(t, moveW, danmuNow)
				canvas.SetColor(color.Black)
				moveW += l + dz
			case 4, 5, 6, 7, 8:
				t = danmakuTypeMap[danItem.Type]
				canvas.SetRGB255(0, 128, 0)
				l, _ = canvas.MeasureString(t)
				canvas.DrawString(t, moveW, danmuNow)
				canvas.SetColor(color.Black)
				moveW += l + dz
			default:
				canvas.SetRGB255(0, 128, 0)
				l, _ = canvas.MeasureString("未知类型" + strconv.Itoa(int(danItem.Type)))
				canvas.DrawString(t, moveW, danmuNow)
				canvas.SetColor(color.Black)
				moveW += l + dz
			}
			if moveW > mcw {
				mcw = moveW
			}
		}
		channelStart = DanmakuStart + float64(len(item.Danmakus)+1)*danmuH
	}
	im := canvas.Image().(*image.RGBA)
	return im.SubImage(image.Rect(0, 0, int(mcw), ch)), nil
}
