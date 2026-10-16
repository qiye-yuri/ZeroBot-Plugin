package chat

import (
	"github.com/FloatTech/zbputils/control"
	"github.com/RomiChan/syncx"
	zero "github.com/wdvxdr1123/ZeroBot"
)

const stateKeyAgentEventSource = zero.StateKeyPrefixKeep + "_chat_ag_event_source__"

// This binding stays in the event context. Only the sequence enters task data.
type agentEventSource struct {
	event                       *zero.Event
	bot, conversation, sequence int64
}

func AgentEventSequence(ctx *zero.Ctx) int64 {
	if ctx == nil || ctx.Event == nil || ctx.Event.PostType != "message" {
		return 0
	}
	mp, ok := ctx.State[control.StateKeySyncxState].(*syncx.Map[string, any])
	if !ok {
		return 0
	}
	raw, ok := mp.Load(stateKeyAgentEventSource)
	if !ok {
		return 0
	}
	source, ok := raw.(agentEventSource)
	conversation := ctx.Event.GroupID
	if conversation == 0 {
		conversation = -ctx.Event.UserID
	}
	if !ok || source.event != ctx.Event || source.bot != ctx.Event.SelfID || source.conversation != conversation || source.sequence <= 0 {
		return 0
	}
	return source.sequence
}
