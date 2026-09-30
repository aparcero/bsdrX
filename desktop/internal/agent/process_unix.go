//go:build !windows

package agent

import (
	"os/exec"
	"syscall"
)

func prepareCommand(cmd *exec.Cmd) {
	// The desktop owns graceful shutdown; terminal signals should not reach the
	// child twice and trigger the C agent's force-exit handler.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func interrupt(cmd *exec.Cmd) error { return cmd.Process.Signal(syscall.SIGTERM) }
