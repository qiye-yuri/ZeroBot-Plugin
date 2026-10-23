package bilibiliparse

import (
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const unifiedHelp = "- B站视频/动态/专栏/直播间链接自动解析\n- >user info [uid或名字]；>vup info [uid或名字]\n- 查成分 [用户]；查弹幕 [用户]；更新vup（主人）\n- 设置b站cookie [Cookie]（主人私聊）\n- 添加b站订阅[uid或名字]；取消b站订阅[用户]\n- 取消b站动态订阅[用户]；取消b站直播订阅[用户]\n- b站推送列表；拉取b站推送（主人或主人定时任务）\n- 开启/关闭B站链接解析；开启/关闭B站动态推送；开启/关闭B站直播推送\n- 查看B站功能开关\n- 开启/关闭视频总结；开启/关闭艾特全体\n- 统一服务开关：bilibiliparse；订阅仍需配置定时拉取"

var featureControl, oldPushControl *ctrl.Control[*zero.Ctx]
var featureBits = map[string]int64{"链接解析": 1 << 8, "动态推送": 1 << 9, "直播推送": 1 << 10}

func scope(ctx *zero.Ctx) int64 {
	if ctx.Event.GroupID != 0 {
		return ctx.Event.GroupID
	}
	return -ctx.Event.UserID
}

// Two bits per option preserve old defaults until explicitly configured.
func featureValue(data, bit int64, fallback bool) bool {
	if data&(bit<<8) == 0 {
		return fallback
	}
	return data&bit != 0
}
func setFeatureValue(data, bit int64, enabled bool) int64 {
	data |= bit << 8
	if enabled {
		return data | bit
	}
	return data &^ bit
}
func featureEnabled(id int64, kind string) bool {
	if featureControl == nil || !featureControl.IsEnabledIn(id) {
		return false
	}
	fallback := true
	if kind != "链接解析" {
		fallback = oldPushControl.IsEnabledIn(id)
	}
	return featureValue(featureControl.GetData(id), featureBits[kind], fallback)
}
func parseEnabled(ctx *zero.Ctx) bool { return featureEnabled(scope(ctx), "链接解析") }

func configureFeatures(en *control.Engine, legacy *ctrl.Control[*zero.Ctx]) {
	featureControl, _ = control.Lookup("bilibiliparse")
	oldPushControl = legacy
	en.OnRegex(`^(开启|关闭)[Bb]站(链接解析|动态推送|直播推送)$`, zero.UserOrGrpAdmin).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		parts := ctx.State["regex_matched"].([]string)
		id := scope(ctx)
		data := setFeatureValue(featureControl.GetData(id), featureBits[parts[2]], parts[1] == "开启")
		if err := featureControl.SetData(id, data); err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.SendChain(message.Text("已", parts[1], "B站", parts[2]))
	})
	en.OnFullMatch("查看B站功能开关").SetBlock(true).Handle(func(ctx *zero.Ctx) {
		text := "B站功能开关："
		for _, kind := range []string{"链接解析", "动态推送", "直播推送"} {
			state := "关闭"
			if featureEnabled(scope(ctx), kind) {
				state = "开启"
			}
			text += "\n" + kind + "：" + state
		}
		ctx.SendChain(message.Text(text))
	})
}
