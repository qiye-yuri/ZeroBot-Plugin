//go:build !windows

package batteryhub

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
