package aiimage

import (
	core "github.com/FloatTech/ZeroBot-Plugin/internal/imagegen"

	"context"
	"fmt"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/chat"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	"github.com/tidwall/gjson"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
	"net/http"
	"strings"
	"time"
)

func init() {
	sdb := &storage{}
	en := control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Extra:            control.ExtraFromString("aiimage"), Brief: "AI画图",
		Help:              "- 设置AI画图接口类型 openai或siliconflow\n- 设置AI画图密钥xxx\n- 清除AI画图密钥\n- 设置AI画图接口地址 完整生成接口URL\n- 设置AI画图模型名 模型ID\n- 查看AI画图配置\n- AI画图 [--size=4k] [--quality=max] [--n=2] 描述\n- AI画图帮助（参数/尺寸/质量/输出/流式/配置/预算/参考图/示例/全部）",
		PrivateDataFolder: "aiimage",
	})
	core.RuntimeAdmin = func(next *core.Update, backup core.Backup) (core.Snapshot, error) {
		if err := sdb.open(en.DataFolder() + "aiimage.db"); err != nil {
			return core.Snapshot{}, err
		}
		return sdb.admin(next, backup)
	}
	getdb := func(ctx *zero.Ctx) bool {
		if err := sdb.open(en.DataFolder() + "aiimage.db"); err != nil {
			ctx.SendChain(message.Text("画图配置读取失败"))
			return false
		}
		return true
	}
	for _, field := range []string{"密钥", "接口地址", "模型名", "接口类型"} {
		en.OnPrefix("设置AI画图"+field, getdb, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
			Handle(func(ctx *zero.Ctx) {
				value := strings.TrimSpace(ctx.State["args"].(string))
				if value == "" {
					ctx.SendChain(message.Text("设置值不能为空；清除密钥请使用：清除AI画图密钥"))
					return
				}
				var err error
				if field == "接口类型" {
					err = sdb.setProtocol(strings.ToLower(value))
				} else {
					if field == "接口地址" && !validImageURL(value) {
						ctx.SendChain(message.Text("请输入完整的http或https图片生成接口地址"))
						return
					}
					err = sdb.updateField(field, value)
				}
				if err != nil {
					ctx.SendChain(message.Text("设置失败: ", err))
					return
				}
				ctx.SendChain(message.Text("成功设置AI画图", field))
			})
	}
	en.OnFullMatch("清除AI画图密钥", getdb, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			if err := sdb.updateField("密钥", ""); err != nil {
				ctx.SendChain(message.Text("清除失败: ", err))
				return
			}
			ctx.SendChain(message.Text("已清除画图密钥"))
		})
	en.OnFullMatch("查看AI画图配置", getdb, zero.OnlyPrivate, zero.SuperUserPermission).SetBlock(true).
		Handle(func(ctx *zero.Ctx) { ctx.SendChain(message.Text(sdb.PrintConfig())) })
	en.OnMessage(func(ctx *zero.Ctx) bool {
		arg, ok := imageCommand(ctx.Event.Message)
		if ok {
			ctx.State["args"] = arg
		}
		return ok
	}).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			prompt := strings.TrimSpace(ctx.State["args"].(string))
			if pages, ok := imageHelp(prompt); ok {
				if len(pages) == 1 {
					ctx.SendChain(message.Text(pages[0]))
					return
				}
				nodes := make(message.Message, 0, len(pages))
				for _, page := range pages {
					nodes = append(nodes, ctxext.BotForwardNode(ctx, message.Text(page)))
				}
				if ctx.Send(nodes).ID() == 0 {
					ctx.SendChain(message.Text("完整帮助合并消息发送失败，请按主题查看。\n\n", pages[0]))
				}
				return
			}
			if !getdb(ctx) {
				return
			}
			if prompt == "" {
				ctx.SendChain(message.Text("请输入图片描述"))
				return
			}
			prompt, referenceCount, selectErr := parseReferenceCount(prompt)
			if selectErr != nil {
				ctx.SendChain(message.Text("画图未开始: ", selectErr))
				return
			}
			conversation := ctx.Event.GroupID
			if conversation == 0 {
				conversation = -ctx.Event.UserID
			}
			run, _ := ctx.State[chat.StateKeyTestRun].(string)
			checkRecall := func() error {
				if e := chat.MessageRecallCheck(ctx.Event.SelfID, conversation, fmt.Sprint(ctx.Event.MessageID), run); e != nil {
					return e
				}
				for _, seg := range ctx.Event.Message {
					if seg.Type == "reply" {
						if e := chat.MessageRecallCheck(ctx.Event.SelfID, conversation, seg.Data["id"], run); e != nil {
							return e
						}
					}
				}
				return nil
			}
			if e := checkRecall(); e != nil {
				ctx.SendChain(message.Text("画图未开始: ", e))
				return
			}
			cutoff := int64(0)
			hasReply := false
			for _, seg := range ctx.Event.Message {

				hasReply = hasReply || seg.Type == "reply"
			}
			if hasReply {
				var err error
				cutoff, err = chat.TestIsolationCutoff(ctx.Event.SelfID)
				if err != nil {
					ctx.SendChain(message.Text("引用隔离状态读取失败，未调用画图模型"))
					return
				}
			}
			sources, err := referenceSources(ctx.Event.Message, ctx.Event.SelfID, ctx.Event.UserID, ctx.Event.GroupID, func(id string) (bool, error) {
				return chat.TestMessageReadAllowed(ctx.Event.SelfID, conversation, id, run)
			}, func(id int64) (gjson.Result, error) {
				rsp := ctx.CallAction("get_msg", zero.Params{"message_id": id})
				if rsp.Status != "ok" || rsp.RetCode != 0 {
					return gjson.Result{}, fmt.Errorf("get_msg failed")
				}
				return rsp.Data, nil
			}, cutoff)
			if err != nil {
				ctx.SendChain(message.Text("画图未开始: ", err))
				return
			}
			sources, err = selectFirstReferences(sources, referenceCount)
			if err != nil {
				ctx.SendChain(message.Text("画图未开始: ", err))
				return
			}
			references, err := prepareReferences(context.Background(), &http.Client{Timeout: 10 * time.Second}, sources, func(file string) referenceSource {
				rsp := ctx.CallAction("get_image", zero.Params{"file": file})
				if rsp.Status != "ok" || rsp.RetCode != 0 {
					return referenceSource{}
				}
				return oneBotReferenceSource(rsp.Data.Get("url").String(), rsp.Data.Get("file").String(), file, ctx.Event.SelfID)
			})
			if err != nil {
				ctx.SendChain(message.Text("画图未开始: ", err))
				return
			}
			if checkRecall() != nil {
				return
			}
			cfg, mode, settings, prompt, reservation, err := sdb.reserve(prompt, time.Now(), references...)
			if err != nil {
				ctx.SendChain(message.Text("画图未开始: ", err))
				return
			}
			if len(references) > 0 {
				ctx.SendChain(message.Text("已读取", len(references), "张参考图，正在交给绘图模型处理..."))
			} else {
				ctx.SendChain(message.Text("正在生成图片，请稍候..."))
			}
			if checkRecall() != nil {
				return
			}
			images, err := generateImageWithPreview(imageHTTPClient, cfg, mode, prompt, settings, func(img string, index int) {
				if checkRecall() == nil {
					ctx.SendChain(message.Text("生成预览（非最终图片） #", index), message.Image(img))
				}
			}, references...)
			accountErr := sdb.finish(reservation, err == nil)
			if checkRecall() != nil {
				return
			}
			if accountErr != nil {
				ctx.SendChain(message.Text("预算状态写入失败，本次预留仍保留。"))
			}
			if err != nil {
				ctx.SendChain(message.Text("画图失败: ", err, "；本次预算预留保留，避免远端已收费却重复放行。"))
				return
			}
			msg := message.Message{message.Text("图片生成完成\n模型: ", cfg.ModelName, "\n质量: ", settings.Quality, "\n尺寸: ", settings.Size, "\n参考图: ", len(references), "张\n最终图片: ", len(images), "张\n预算占用: $", dollars(reservation.Amount))}
			for _, img := range images {
				msg = append(msg, message.Image(img))
			}
			if ctx.Send(msg).ID() == 0 {
				ctx.SendChain(message.Text("图片已生成，但发送失败；请检查OneBot连接与图片发送权限。"))
			}
		})
}
