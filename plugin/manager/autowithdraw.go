package manager

import (
	"github.com/FloatTech/floatbox/process"
	"github.com/FloatTech/zbputils/control"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

func recallScope(ctx *zero.Ctx) int64 {
	if ctx.Event.GroupID != 0 {
		return ctx.Event.GroupID
	}
	return -ctx.Event.UserID
}

// Keep the legacy control table, but expose only the manager service.
func configureAutoWithdraw(engine *control.Engine) {
	notices, legacy := engine.Component("autowithdraw", nil)
	// An explicit component setting overrides inherited legacy global defaults.
	// The old component had no custom data bits; bit 1 marks a configured value.
	enabled := func(id int64) bool {
		data := legacy.GetData(id)
		if data&2 != 0 {
			return data&1 != 0
		}
		return legacy.IsEnabledIn(id)
	}
	engine.OnRegex(`^(开启|关闭)自动撤回$`, zero.UserOrGrpAdmin).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		action := ctx.State["regex_matched"].([]string)[1]
		id := recallScope(ctx)
		data := legacy.GetData(id) | 2
		if action == "开启" {
			data |= 1
		} else {
			data &^= 1
		}
		err := legacy.SetData(id, data)
		if err != nil {
			ctx.SendChain(message.Text("ERROR: ", err))
			return
		}
		ctx.SendChain(message.Text("已", action, "自动撤回"))
	})
	engine.OnFullMatch("查看群管功能开关").SetBlock(true).Handle(func(ctx *zero.Ctx) {
		state := "关闭"
		if enabled(recallScope(ctx)) {
			state = "开启"
		}
		ctx.SendChain(message.Text("群管功能开关：\n自动撤回：", state))
	})
	notices.OnNotice(func(ctx *zero.Ctx) bool {
		return (ctx.Event.NoticeType == "group_recall" || ctx.Event.NoticeType == "friend_recall") && enabled(recallScope(ctx))
	}).SetBlock(false).Handle(func(ctx *zero.Ctx) {
		id, ok := ctx.Event.MessageID.(int64)
		if !ok {
			return
		}
		for _, msg := range zero.GetTriggeredMessages(message.NewMessageIDFromInteger(id)) {
			process.SleepAbout1sTo2s()
			ctx.DeleteMessage(msg)
		}
	})
}
