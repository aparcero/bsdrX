package agent

import (
	"os/exec"
	"syscall"
)

func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// Windows does not implement os.Interrupt for child processes without a console.
func interrupt(cmd *exec.Cmd) error { return cmd.Process.Kill() }
