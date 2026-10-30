package ctb

import (
	"context"
	"fmt"
)

// Cancel requests cancellation without releasing the running task's lock/state.
// The QQ caller must authorize the owner before entering this method.
func (s *Service) Cancel(groupID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.busy {
		return false, nil
	}
	if s.cancel == nil {
		return false, fmt.Errorf("当前是文件重发阶段，不能用FTB取消停止QQ上传")
	}
	if groupID > 0 && groupID != s.activeGroup {
		return false, fmt.Errorf("当前任务不来自本群，请在来源群或主人私聊取消")
	}
	s.cancel()
	return true, nil
}

func taskContextError(ctx context.Context, dir string) error {
	text := "任务超时"
	if ctx.Err() == context.Canceled {
		text = "任务已取消"
	}
	if dir != "" {
		text += "；部分下载保留于 " + dir
	}
	return fmt.Errorf("%s", text)
}
