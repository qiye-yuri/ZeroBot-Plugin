// Package userselect resolves Bilibili users without letting AI consume menu replies.
package userselect

import (
	"strconv"
	"strings"
	"sync"
	"time"

	bz "github.com/FloatTech/AnimeAPI/bilibili"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var active sync.Map

func RequireUser(cfg *bz.CookieConfig) func(*zero.Ctx) bool {
	return func(ctx *zero.Ctx) bool {
		// A matched command remains consumed on cancellation/search failure.
		ctx.GetMatcher().Break = true
		keyword := strings.TrimSpace(ctx.State["regex_matched"].([]string)[1])
		search := func() bool {
			results, err := searchUser(cfg, keyword)
			if err != nil {
				ctx.SendChain(message.Text("ERROR: ", err))
				return false
			}
			if len(results) == 0 {
				ctx.SendChain(message.Text("未找到匹配的B站用户"))
				return false
			}
			ctx.State["uid"] = strconv.FormatInt(results[0].Mid, 10)
			return true
		}
		if _, err := strconv.ParseUint(keyword, 10, 64); err != nil {
			return search()
		}
		key := [2]int64{ctx.Event.GroupID, ctx.Event.UserID}
		if _, busy := active.LoadOrStore(key, true); busy {
			ctx.SendChain(message.Text("已有B站用户选择正在进行，请先选择或发送“取消”"))
			return false
		}
		defer active.Delete(key)
		// Component priority may be below AI; consume at priority 1, after history hooks (0).
		next := zero.NewFutureEvent("message", 1, true, func(c *zero.Ctx) bool {
			return c.Event.GroupID == key[0] && c.Event.UserID == key[1] && c.Event.UserID != c.Event.SelfID
		})
		recv, cancel := next.Repeat()
		var cleanup sync.Once
		finish := func() { cleanup.Do(cancel) }
		defer finish()
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		ctx.SendChain(message.Text("输入为纯数字, 请选择查询uid还是用户名, 输入对应序号：\n0. 查询uid\n1. 查询用户名\n发送“取消”结束"))
		for {
			select {
			case <-timer.C:
				ctx.SendChain(message.Text("选择超时，自动按UID查询"))
				ctx.State["uid"] = keyword
				return true
			case c := <-recv:
				switch strings.TrimSpace(c.ExtractPlainText()) {
				case "0":
					ctx.State["uid"] = keyword
					return true
				case "1":
					// Release the menu before an upstream search that may take time.
					finish()
					timer.Stop()
					return search()
				case "取消":
					ctx.SendChain(message.Text("已取消B站用户查询"))
					return false
				default:
					ctx.SendChain(message.Text("请输入0、1，或发送“取消”"))
				}
			}
		}
	}
}
