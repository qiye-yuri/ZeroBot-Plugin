package mcfish

import (
	zero "github.com/wdvxdr1123/ZeroBot"
	"sync"
	"time"
)

// Each phase consumes replies only in its initiating session, ahead of AI chat.
// Call stop as soon as the phase finishes, in addition to deferring it on errors.
func gameReply(ctx *zero.Ctx, pattern string, timeout time.Duration) (<-chan *zero.Ctx, func(), <-chan time.Time) {
	next := ctx.FutureEvent("message", zero.RegexRule(pattern), ctx.CheckSession())
	next.Block = true
	recv, cancel := next.Repeat()
	timer := time.NewTimer(timeout)
	var once sync.Once
	return recv, func() { once.Do(func() { cancel(); timer.Stop() }) }, timer.C
}
