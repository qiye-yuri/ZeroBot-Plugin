package ctb

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	core "github.com/FloatTech/ZeroBot-Plugin/internal/ctb"
	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var engine = control.AutoRegister(&ctrl.Options[*zero.Ctx]{Brief: "FTB整合包查询和下载", Help: "FTB搜索 关键词 / FTB查询 整合包ID / FTB下载 整合包ID [版本ID或latest] [light/full] / FTB重发 任务编号（仅主人，不重新下载或扣下载次数） / FTB工具状态。私聊仅主人，群聊需@机器人，权限、每日下载次数和群文件/私聊发送位置可在后台FTB工具调整。"})

func init() {
	engine.OnRegex(`(?i)^FTB(搜索|查询|下载|重发|工具状态)(?:\s+(.*))?$`).SetBlock(true).Handle(handle)
}

func handle(ctx *zero.Ctx) {
	group := ctx.Event.GroupID > 0
	owner := zero.SuperUserPermission(ctx)
	if !group && !owner {
		return
	}
	if group && !zero.OnlyToMe(ctx) {
		return
	}
	m := ctx.State["regex_matched"].([]string)
	action := map[string]string{"搜索": "search", "查询": "inspect", "下载": "download", "重发": "resend", "工具状态": "status"}[m[1]]
	if action == "resend" && !owner {
		ctx.SendChain(message.Text("FTB重发仅限机器人主人操作，请联系主人；不需要重新下载。"))
		return
	}
	snap, e := core.Default.Settings("", nil)
	if e != nil {
		ctx.SendChain(message.Text("配置读取失败。下一步：请主人检查后台FTB工具。"))
		return
	}
	if !core.CanUse(group, zero.OnlyToMe(ctx), owner, zero.AdminPermission(ctx), snap.Config.GroupAccess, action) {
		if group && snap.Config.GroupAccess != "" && snap.Config.GroupAccess != "disabled" {
			ctx.SendChain(message.Text("当前群聊权限不允许此操作。下一步：请主人在后台FTB工具调整群聊权限。"))
		}
		return
	}
	if action == "status" {
		ctx.SendChain(message.Text(fmt.Sprintf("启用：%t\n忙碌：%t\n群聊权限：%s\n群聊文件发送：%s\n每群每天最多发起%d次下载（失败也计次，北京时间零点重置）\n下一步：发送 FTB搜索 Evolution；群聊请先@机器人。", snap.Config.Enabled, snap.Busy, snap.Config.GroupAccess, snap.Config.GroupDelivery, snap.Config.GroupDailyLimit)))
		return
	}
	if action == "resend" {
		task := strings.TrimSpace(m[2])
		if task == "" {
			ctx.SendChain(message.Text("格式：FTB重发 下载完成消息中的任务编号。仅重发已登记的ZIP，不重新下载、不扣群下载次数。"))
			return
		}
		go func() {
			uploadCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			err := core.Default.Resend(task, ctx.Event.UserID, ctx.Event.GroupID, func(action string, params map[string]any) error {
				reply := ctx.CallActionWithContext(uploadCtx, action, zero.Params(params))
				if reply.Status != "ok" || reply.RetCode != 0 {
					return fmt.Errorf("QQ文件发送接口未确认成功（状态%s，代码%d）", reply.Status, reply.RetCode)
				}
				return nil
			})
			if err != nil {
				ctx.SendChain(message.Text("FTB重发失败：", err, "。未重新下载、未扣下载次数；已有文件保留。"))
				return
			}
			ctx.SendChain(message.Text("QQ接口已接受重发，未重新下载、未扣下载次数。请按当前发送设置在本群文件或与机器人的私聊确认收到；接口成功不代表已收件。"))
		}()
		return
	}
	r := core.Request{Action: action, Pack: strings.TrimSpace(m[2])}
	if action == "download" {
		f := strings.Fields(m[2])
		if len(f) < 1 || len(f) > 3 {
			ctx.SendChain(message.Text("格式：FTB下载 整合包ID [版本ID或latest] [light/full]\n下一步：例如 FTB下载 135 latest light；先用FTB查询 135查看版本。"))
			return
		}
		r.Pack = f[0]
		r.Mode = "light"
		if len(f) > 1 && strings.ToLower(f[1]) != "latest" {
			r.Version = f[1]
		}
		if len(f) > 2 {
			r.Mode = strings.ToLower(f[2])
		}
	}
	if _, e = core.Args(r); e != nil {
		ctx.SendChain(message.Text(e, "\n下一步：搜索用FTB搜索 Evolution，版本和下载参数请先用FTB查询 135核对。"))
		return
	}
	if !snap.Config.Enabled {
		ctx.SendChain(message.Text("FTB工具未启用。下一步：请主人在后台FTB工具开启。"))
		return
	}
	if action == "download" {
		destination := "私聊文件"
		if group && snap.Config.GroupDelivery == "group" {
			destination = "本群文件"
		}
		delivery := "下载后保留本地文件，自动发送未开启。"
		if snap.Config.Upload {
			delivery = "校验完成后发送到" + destination + "。"
		}
		quota := ""
		if group {
			quota = fmt.Sprintf("本群每天最多发起%d次下载，开始执行后即计次，失败也计次。", snap.Config.GroupDailyLimit)
		}
		ctx.SendChain(message.Text("正在准备下载并校验ZIP。", delivery, quota, "\n下一步：等待完成提示，请勿重复提交。"))
	} else {
		ctx.SendChain(message.Text("正在查询FTB。下一步：等待结果后按提示选择整合包或版本。"))
	}
	go func() {
		res, err := core.Default.RunFor(context.Background(), r, ctx.Event.GroupID)
		if err != nil {
			detail := err.Error()
			if group {
				detail = strings.ReplaceAll(detail, snap.Config.OutputDir, "本机下载目录")
				detail = strings.ReplaceAll(detail, snap.Config.Executable, "FTB工具")
			}
			ctx.SendChain(message.Text("FTB任务失败：", detail, "\n下一步：根据错误提示处理；额度用尽请明天再试，忙碌请等待，网络或程序错误请主人查看后台。"))
			return
		}
		if action != "download" {
			ctx.SendChain(message.Text(res.Text, "\n\n", core.NextStep(r), "\n群聊操作请@机器人。"))
			return
		}
		names := make([]string, len(res.Files))
		for i, p := range res.Files {
			names[i] = filepath.Base(p)
		}
		completed := "下载并校验完成：\n" + strings.Join(names, "\n")
		if !group {
			completed = res.Text
		}
		completed += "\n重发任务编号：" + res.TaskID + "\n需要再次发送时请主人发送 FTB重发 " + res.TaskID
		if !res.Upload {
			ctx.SendChain(message.Text(completed, "\n自动发送未开启。下一步：请主人在后台FTB工具开启下载后发送文件，或从本机下载目录取得ZIP。"))
			return
		}
		ctx.SendChain(message.Text(completed, "\n正在发送文件。下一步：等待QQ发送结果。"))
		uploadCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		err = core.Deliver(res.Files, ctx.Event.UserID, ctx.Event.GroupID, snap.Config.GroupDelivery, func(action string, params map[string]any) error {
			reply := ctx.CallActionWithContext(uploadCtx, action, zero.Params(params))
			if reply.Status != "ok" || reply.RetCode != 0 {
				return fmt.Errorf("QQ文件发送接口未确认成功（状态%s，代码%d）", reply.Status, reply.RetCode)
			}
			return nil
		})
		if err != nil {
			ctx.SendChain(message.Text(err, "。文件已保留本机。\n下一步：群文件请检查机器人群文件权限及空间；私聊请先添加机器人好友。修复后请主人发送 FTB重发 ", res.TaskID, "，避免重复下载消耗次数。"))
			return
		}
		destination := "与机器人的私聊"
		if group && snap.Config.GroupDelivery == "group" {
			destination = "本群聊天及群文件"
		}
		ctx.SendChain(message.Text("QQ接口已接受文件发送。下一步：打开", destination, "确认文件并下载；接口成功不代表已确认你收到。\n", core.NextStep(r)))
	}()
}
