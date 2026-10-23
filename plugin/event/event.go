// Package event 好友申请以及群聊邀请事件处理
package event

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/FloatTech/zbputils/ctxext"
	base14 "github.com/fumiama/go-base16384"
	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

func init() {
	engine := control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "好友申请和群聊邀请事件处理",
		Help: "- [开启|关闭]自动同意[申请|邀请|主人]\n" +
			"- [同意|拒绝][申请|邀请][flag]\n- 查看申请处理设置\n" +
			"Tips: 信息默认发送给主人列表第一位, 默认同意所有主人的事件, flag跟随事件一起发送",
	})
	engine.On("request/group/invite", hasOwner).SetBlock(false).
		Handle(func(ctx *zero.Ctx) {
			c, ok := ctx.State["manager"].(*ctrl.Control[*zero.Ctx])
			if ok {
				su := zero.BotConfig.SuperUsers[0]
				now := time.Unix(ctx.Event.Time, 0).Format("2006-01-02 15:04:05")
				es := encodeFlag("邀请", ctx.Event.Flag)
				userid := ctx.Event.UserID
				username := ctx.GetStrangerInfo(userid, false).Get("nickname").String()
				data := (storage)(c.GetData(-su))
				groupname := ctx.GetThisGroupInfo(true).Name
				groupid := ctx.Event.GroupID
				logrus.Info("[event]收到来自[", username, "](", userid, ")的群聊邀请，群:[", groupname, "](", groupid, ")")
				if data.isinviteon() || (!data.ismasteroff() && zero.SuperUserPermission(ctx)) {
					if err := handleRequest(ctx, "邀请", ctx.Event.Flag, true, ""); err != nil {
						ctx.SendPrivateMessage(su, message.Message{message.Text("自动同意失败: ", err, "\n可重试：同意邀请", es)})
						return
					}
					ctx.SendPrivateForwardMessage(su, message.Message{message.CustomNode(ctxext.BotName(), ctx.Event.SelfID,
						"已自动同意在"+now+"收到来自"+
							"\n用户:["+username+"]("+strconv.FormatInt(userid, 10)+")的群聊邀请"+
							"\n群聊:["+groupname+"]("+strconv.FormatInt(groupid, 10)+")"+
							"\nflag:"+es)})
					return
				}
				ctx.SendPrivateForwardMessage(su,
					message.Message{message.CustomNode(ctxext.BotName(), ctx.Event.SelfID,
						"在"+now+"收到来自"+
							"\n用户:["+username+"]("+strconv.FormatInt(userid, 10)+")的群聊邀请"+
							"\n群聊:["+groupname+"]("+strconv.FormatInt(groupid, 10)+")"+
							"\n请在下方复制flag并在前面加上:"+
							"\n同意/拒绝邀请，来决定同意还是拒绝"),
						message.CustomNode(ctxext.BotName(), ctx.Event.SelfID, es)})
			}
		})
	engine.On("request/friend", hasOwner).SetBlock(false).
		Handle(func(ctx *zero.Ctx) {
			c, ok := ctx.State["manager"].(*ctrl.Control[*zero.Ctx])
			if ok {
				su := zero.BotConfig.SuperUsers[0]
				now := time.Unix(ctx.Event.Time, 0).Format("2006-01-02 15:04:05")
				es := encodeFlag("申请", ctx.Event.Flag)
				comment := ctx.Event.Comment
				userid := ctx.Event.UserID
				username := ctx.GetStrangerInfo(userid, false).Get("nickname").String()
				data := (storage)(c.GetData(-su))
				logrus.Info("[event]收到来自[", username, "](", userid, ")的好友申请")
				if data.isapplyon() || (!data.ismasteroff() && zero.SuperUserPermission(ctx)) {
					if err := handleRequest(ctx, "申请", ctx.Event.Flag, true, ""); err != nil {
						ctx.SendPrivateMessage(su, message.Message{message.Text("自动同意失败: ", err, "\n可重试：同意申请", es)})
						return
					}
					ctx.SendPrivateForwardMessage(su, message.Message{message.CustomNode(ctxext.BotName(), ctx.Event.SelfID,
						"已自动同意在"+now+"收到来自"+
							"\n用户:["+username+"]("+strconv.FormatInt(userid, 10)+")"+
							"\n的好友请求:"+comment+
							"\nflag:"+es)})
					return
				}
				ctx.SendPrivateForwardMessage(su,
					message.Message{message.CustomNode(ctxext.BotName(), ctx.Event.SelfID,
						"在"+now+"收到来自"+
							"\n用户:["+username+"]("+strconv.FormatInt(userid, 10)+")"+
							"\n的好友请求:"+comment+
							"\n请在下方复制flag并在前面加上:"+
							"\n同意/拒绝申请，来决定同意还是拒绝"),
						message.CustomNode(ctxext.BotName(), ctx.Event.SelfID, es)})
			}
		})
	engine.OnRegex(`^(同意|拒绝)(申请|邀请)\s*(\S+)\s*(.*)$`, hasOwner, zero.SuperUserPermission, zero.OnlyPrivate).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			cmd := ctx.State["regex_matched"].([]string)[1]
			org := ctx.State["regex_matched"].([]string)[2]
			es := ctx.State["regex_matched"].([]string)[3]
			other := ctx.State["regex_matched"].([]string)[4]
			flag, err := decodeFlag(org, es)
			if err == nil {
				err = handleRequest(ctx, org, flag, cmd == "同意", other)
			}
			if err != nil {
				ctx.SendChain(message.Text("处理失败: ", err))
				return
			}
			ctx.SendChain(message.Text("已", cmd, org))
		})
	engine.OnRegex(`^(开启|关闭)自动同意(申请|邀请|主人)$`, hasOwner, zero.SuperUserPermission, zero.OnlyPrivate).SetBlock(true).
		Handle(func(ctx *zero.Ctx) {
			c := ctx.State["manager"].(*ctrl.Control[*zero.Ctx])
			su := zero.BotConfig.SuperUsers[0]
			option := ctx.State["regex_matched"].([]string)[1]
			from := ctx.State["regex_matched"].([]string)[2]
			data := (storage)(c.GetData(-su))
			switch from {
			case "申请":
				data.setapply(option == "开启")
			case "邀请":
				data.setinvite(option == "开启")
			case "主人":
				data.setmaster(option == "关闭")
			}
			err := c.SetData(-su, int64(data))
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return
			}
			ctx.SendChain(message.Text("已设置自动同意" + from + "为" + option))
		})
	engine.OnFullMatch("查看申请处理设置", hasOwner, zero.SuperUserPermission, zero.OnlyPrivate).SetBlock(true).Handle(func(ctx *zero.Ctx) {
		c := ctx.State["manager"].(*ctrl.Control[*zero.Ctx])
		data := storage(c.GetData(-zero.BotConfig.SuperUsers[0]))
		label := func(on bool) string {
			if on {
				return "开启"
			}
			return "关闭"
		}
		ctx.SendChain(message.Text("自动同意申请：", label(data.isapplyon()), "\n自动同意邀请：", label(data.isinviteon()), "\n自动同意主人：", label(!data.ismasteroff())))
	})

}

// Flags are opaque OneBot identifiers; preserve every byte and bind the kind.
func encodeFlag(kind, flag string) string {
	prefix := "f_"
	if kind == "邀请" {
		prefix = "g_"
	}
	return prefix + base64.RawURLEncoding.EncodeToString([]byte(flag))
}
func decodeFlag(kind, token string) (string, error) {
	prefix := "f_"
	if kind == "邀请" {
		prefix = "g_"
	}
	if strings.HasPrefix(token, prefix) {
		raw, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(token, prefix))
		if err == nil && len(raw) > 0 {
			return string(raw), nil
		}
	} else if len([]rune(token)) == 4 {
		valid := true
		for _, r := range token {
			if r < '一' || r > '踀' {
				valid = false
			}
		}
		if valid {
			var buf [8]byte
			copy(buf[1:], base14.DecodeFromString(token))
			return strconv.FormatUint(binary.BigEndian.Uint64(buf[:]), 10), nil
		}
	}
	return "", fmt.Errorf("flag无效或申请类型不匹配，请复制对应通知中的完整flag")
}
func hasOwner(_ *zero.Ctx) bool { return len(zero.BotConfig.SuperUsers) > 0 }
func handleRequest(ctx *zero.Ctx, kind, flag string, approve bool, other string) error {
	action := "set_friend_add_request"
	params := zero.Params{"flag": flag, "approve": approve, "remark": other}
	if kind == "邀请" {
		action = "set_group_add_request"
		params = zero.Params{"flag": flag, "sub_type": "invite", "approve": approve, "reason": other}
	}
	result := ctx.CallAction(action, params)
	if result.Status != "ok" || result.RetCode != 0 {
		return fmt.Errorf("OneBot未确认成功（status=%s, retcode=%d）", result.Status, result.RetCode)
	}
	return nil
}
