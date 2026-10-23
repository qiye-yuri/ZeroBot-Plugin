// Package aichatcfg aichat 的配置, 优先级要比 aichat 高
package aichatcfg

import (
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"

	"github.com/FloatTech/ZeroBot-Plugin/internal/aicenter"
	"github.com/FloatTech/zbputils/chat"

	"github.com/FloatTech/zbputils/ctxext"
)

var en = aicenter.SettingsEngine

func init() {
	en.UsePreHandler(chat.EnsureConfig, func(ctx *zero.Ctx) bool {
		k := zero.StateKeyPrefixKeep + "aichatcfg_stor__"
		if _, ok := ctx.State[k]; ok {
			return true
		}
		gid := ctx.Event.GroupID
		if gid == 0 {
			gid = -ctx.Event.UserID
		}
		stor, err := chat.NewStorage(ctx, gid)
		if err != nil {
			logrus.Warnln("ERROR: ", err)
			return false
		}
		ctx.State[k] = stor
		return true
	})
	en.OnFullMatchGroup([]string{"开启AI主动接话", "关闭AI主动接话"}, zero.OnlyGroup, zero.AdminPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		value := int64(0)
		if ctx.ExtractPlainText() == "开启AI主动接话" {
			value = 1
		}
		err := ctxext.UpdateStorage(ctx, ctx.Event.GroupID, func(s ctxext.Storage) ctxext.Storage { return s.Set(value, chat.BitmapProactive) })
		if err != nil {
			ctx.SendChain(message.Text("设置失败，原配置保留。"))
			return
		}
		ctx.SendChain(message.Text("主动接话设置已保存。间隔和续聊窗口可通过“设置AI主动接话间隔10”“设置AI续聊窗口2”调整，单位分钟。"))
	})
	en.OnPrefix("设置AI主动接话间隔", zero.OnlyGroup, zero.AdminPermission).SetBlock(true).
		Handle(ctxext.NewStorageSaveBitmapHandler(chat.BitmapProactiveMinutes, 1, 1440))
	en.OnPrefix("设置AI续聊窗口", zero.OnlyGroup, zero.AdminPermission).SetBlock(true).
		Handle(ctxext.NewStorageSaveBitmapHandler(chat.BitmapFollowupMinutes, 1, 30))
	en.OnPrefix("设置AI聊天触发概率", zero.AdminPermission).SetBlock(true).
		Handle(ctxext.NewStorageSaveBitmapHandler(chat.BitmapRate, 0, 100))
	en.OnPrefix("设置AI聊天温度", zero.AdminPermission).SetBlock(true).
		Handle(ctxext.NewStorageSaveBitmapHandler(chat.BitmapTemp, 0, 100))
	en.OnPrefix("设置AI聊天接口类型", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetModelType("Type"))
	en.OnPrefix("设置AI聊天识图接口类型", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetModelType("ImageType"))
	en.OnPrefix("设置AI聊天Agent接口类型", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetModelType("AgentType"))
	en.OnPrefix("设置AI聊天接口地址", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("API"))
	en.OnPrefix("设置AI聊天识图接口地址", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("ImageAPI"))
	en.OnPrefix("设置AI聊天Agent接口地址", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("AgentAPI"))
	en.OnPrefix("设置AI聊天密钥", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("Key"))
	en.OnPrefix("设置AI聊天识图密钥", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("ImageKey"))
	en.OnPrefix("设置AI聊天Agent密钥", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("AgentKey"))
	en.OnPrefix("设置AI聊天模型名", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("ModelName"))
	en.OnPrefix("设置AI聊天识图模型名", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("ImageModelName"))
	en.OnPrefix("设置AI聊天Agent模型名", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("AgentModelName"))
	en.OnPrefix("设置AI聊天系统提示词", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("SystemP"))
	en.OnPrefix("设置AI聊天Agent性格", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("AgentChar"))
	en.OnPrefix("设置AI聊天Agent性别", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("AgentSex"))
	en.OnFullMatch("查看AI聊天系统提示词", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		ctx.SendChain(message.Text(chat.ConfigOf(ctx).Redact(chat.ConfigOf(ctx).SystemP)))
	})
	en.OnFullMatch("查看AI聊天Agent性格", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		ctx.SendChain(message.Text(chat.ConfigOf(ctx).Redact(chat.ConfigOf(ctx).AgentChar)))
	})
	en.OnFullMatch("重置AI聊天系统提示词", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).Handle(chat.ResetSystemPrompt)
	en.OnFullMatch("重置AI聊天Agent性格性别", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).Handle(chat.ResetAgentCharacter)
	en.OnPrefix("设置AI聊天分隔符", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("Separator"))
	en.OnRegex("^设置AI聊天(不)?响应AT$", zero.SuperUserPermission).SetBlock(true).
		Handle(ctxext.NewStorageSaveBoolHandler(chat.BitmapNrat))
	en.OnRegex("^设置AI聊天(不)?支持系统提示词$", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetBool("NoSystemP"))
	en.OnRegex("^设置AI聊天(不)?使用Agent模式$", zero.SuperUserPermission).SetBlock(true).
		Handle(ctxext.NewStorageSaveBoolHandler(chat.BitmapNagt))
	en.OnPrefix("设置AI聊天最大长度", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetUint("MaxN"))
	en.OnPrefix("设置AI聊天TopP", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetFloat32("TopP"))
	en.OnRegex("^设置AI聊天(不)?以AI语音输出$", zero.AdminPermission).SetBlock(true).
		Handle(ctxext.NewStorageSaveBoolHandler(chat.BitmapNrec))
	en.OnPrefix("设置AI聊天努力度", chat.EnsureConfig, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(chat.NewExtraSetStr("ReasoningEffort"))
	en.OnFullMatch("查看AI聊天配置", chat.EnsureConfig, zero.SuperUserPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			gid := ctx.Event.GroupID
			if gid == 0 {
				gid = -ctx.Event.UserID
			}
			stor, err := chat.NewStorage(ctx, gid)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(
				message.Text(
					"【当前AI聊天本群配置】\n",
					"• 触发概率：", int(stor.Rate()), "\n",
					"• 温度：", stor.Temp(), "\n",
					"• 以AI语音输出：", chat.ModelBool(!stor.NoRecord()), "\n",
					"• 使用Agent：", chat.ModelBool(!stor.NoAgent()), "\n",
					"• 响应@：", chat.ModelBool(!stor.NoReplyAt()), "\n",
					"• 群主动接话：", chat.ModelBool(stor.Proactive()), "\n",
					"• 主动接话间隔：", stor.ProactiveMinutes(), "分钟\n",
					"• 相关话题续聊窗口：", stor.FollowupMinutes(), "分钟\n",
				),
				message.Text("【当前AI聊天全局配置】\n", chat.ConfigOf(ctx)),
			)
		})
	en.OnFullMatch("重置AI聊天Agent", chat.EnsureConfig, zero.SuperUserPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		if err := chat.ResetAgents(); err != nil {
			ctx.SendChain(message.Text("重置失败，请检查本机对话记录存储。"))
			return
		}
		ctx.SendChain(message.Text("已开启新的 Agent 上下文，历史记录仍保留在本机。"))
	})
	en.OnFullMatch("重置AI聊天", chat.EnsureConfig, zero.SuperUserPermission).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		chat.ResetChat()
		ctx.SendChain(message.Text("成功"))
	})
}
