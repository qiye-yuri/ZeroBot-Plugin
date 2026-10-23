// Package aicenter owns the single user-facing AI service and legacy data.
package aicenter

import (
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	zero "github.com/wdvxdr1123/ZeroBot"
)

var Engine = control.Register("aichat", &ctrl.Options[*zero.Ctx]{
	Extra: control.ExtraFromString("aichat"),
	Brief: "AI中心：聊天、Agent、GPT与群聊总结",
	Help: "私聊直接发送文字；群内@机器人并发送文字。\n" +
		"- /gpt [问题]（支持引用消息）\n- 群聊总结 [消息数目]（群管理员或主人）\n" +
		"- 查看AI聊天配置（主人）\n- 设置AI聊天触发概率[0-100]（管理员）\n" +
		"- 设置AI聊天(不)使用Agent模式（主人）\n" +
		"- 开启AI主动接话 / 关闭AI主动接话（群管理员）\n" +
		"- 设置AI主动接话间隔10 / 设置AI续聊窗口2（分钟）\n" +
		"模型、接口、人物设定及会话参数可在后台AI设置中管理。\n" +
		"自然聊天受响应AT、触发概率和Agent配置影响；/gpt为明确命令入口。",
	PrivateDataFolder: "aichat",
})

// These controls preserve existing settings without registering old plugins.
var SettingsEngine, Settings = Engine.Component("aichatcfg", &ctrl.Options[*zero.Ctx]{Extra: control.ExtraFromString("aichat")})
var CommandsEngine, Commands = Engine.Component("llm", nil)
