package clipboard

import (
	"os/exec"
	"syscall"
)

// hide keeps a console window from flashing up while pasting.
func hide(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
