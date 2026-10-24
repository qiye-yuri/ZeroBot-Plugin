//go:build windows

package ctb

import (
	"os/exec"
	"syscall"
)

func hideWindow(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
