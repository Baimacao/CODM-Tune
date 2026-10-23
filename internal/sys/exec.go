package sys

import (
	"os/exec"
	"syscall"
)

// HideWindow prevents helper consoles (powercfg, reg, wevtutil) from flashing.
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

func hideWindow(cmd *exec.Cmd) { HideWindow(cmd) }

