// Package breakrepeat 打断复读
package breakrepeat

import (
	"math/rand"
	"strconv"
	"sync"

	ctrl "github.com/FloatTech/zbpctrl"
	"github.com/FloatTech/zbputils/control"
	"github.com/RomiChan/syncx"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

const throttle = 3 // 不可超过 9

var sm syncx.Map[int64, string]
var repeatMu sync.Mutex

func init() {
	engine := control.AutoRegister(&ctrl.Options[*zero.Ctx]{
		DisableOnDefault: false,
		Brief:            "打断复读",
		Help:             "- 群内连续5条相同文字时打断复读；图片不参与\n",
	})
	engine.On("message/group", zero.OnlyGroup, func(ctx *zero.Ctx) bool {
		// Images without a public URL are still images (e.g. file-only events).
		for _, segment := range ctx.Event.Message {
			if segment.Type == "image" {
				return false
			}
		}
		return true
	}).SetBlock(false).
		Handle(func(ctx *zero.Ctx) {
			repeatMu.Lock()
			defer repeatMu.Unlock()
			gid := ctx.Event.GroupID
			raw := ctx.Event.RawMessage
			r, ok := sm.Load(gid)
			if !ok || len(r) <= 3 || r[3:] != raw {
				sm.Store(gid, "0: "+raw)
				return
			}
			c := int(r[0] - '0')
			if c < throttle {
				sm.Store(gid, strconv.Itoa(c+1)+": "+raw)
				return
			}
			sm.Delete(gid)
			r = raw
			if len([]rune(raw)) > 1 {
				ru := []rune(raw)
				rand.Shuffle(len(ru), func(i, j int) {
					ru[i], ru[j] = ru[j], ru[i]
				})
				r = string(ru)
			}
			ctx.SendChain(message.Text("打断复读：", r))
		})
}
