//go:build !windows

package ctb

import "os/exec"

func hideWindow(c *exec.Cmd) {}
