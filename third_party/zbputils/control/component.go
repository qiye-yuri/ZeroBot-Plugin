package control

import (
	ctrl "github.com/FloatTech/zbpctrl"
	zero "github.com/wdvxdr1123/ZeroBot"
)

// Component adds handlers controlled by the parent's switch while retaining
// an existing data namespace. The namespace is not a separately registered
// plugin and cannot expose an independent enable/disable switch.
// Call only during package initialization, before serving events.
func (e *Engine) Component(namespace string, options *ctrl.Options[*zero.Ctx]) (*Engine, *ctrl.Control[*zero.Ctx]) {
	if _, exists := managers.Lookup(namespace); exists {
		panic("component namespace already registered: " + namespace)
	}
	data := managers.NewControl(namespace, options)
	managers.Lock()
	delete(managers.M, namespace)
	managers.Unlock()
	parent, ok := managers.Lookup(e.service)
	if !ok {
		panic("component parent missing: " + e.service)
	}
	child := &Engine{en: zero.New(), prio: e.prio - 1, service: e.service, datafolder: e.datafolder}
	if p, found := custpriomap[namespace]; found {
		child.prio = int(p)
	}
	child.en.UsePreHandler(func(ctx *zero.Ctx) bool {
		if ctx.Event.PostType == "message" && ctx.Event.UserID == ctx.Event.SelfID {
			return false
		}
		ctx.State["manager"] = data
		return parent.Handler(ctx.Event.GroupID, ctx.Event.UserID)
	}, addsyncxstate)
	return child, data
}
