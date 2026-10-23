package ctxext

import (
	zero "github.com/wdvxdr1123/ZeroBot"
	"testing"
	"time"
)

func TestDefaultLimitChangeAppliesToExistingUser(t *testing.T) {
	defer SetDefaultLimiterManagerParam(10*time.Second, 5)
	ctx := &zero.Ctx{Event: &zero.Event{UserID: 98765, GroupID: 54321}}
	SetDefaultLimiterManagerParam(time.Hour, 1)
	if !LimitByUser(ctx).Acquire() || LimitByUser(ctx).Acquire() {
		t.Fatal("initial limit ineffective")
	}
	SetDefaultLimiterManagerParam(20*time.Millisecond, 2)
	if !LimitByUser(ctx).Acquire() || !LimitByUser(ctx).Acquire() || LimitByUser(ctx).Acquire() {
		t.Fatal("existing user did not receive new limit")
	}
	time.Sleep(30 * time.Millisecond)
	if !LimitByUser(ctx).Acquire() {
		t.Fatal("token did not recover")
	}
	if !LimitByGroup(ctx).Acquire() {
		t.Fatal("group limiter unavailable")
	}
}
