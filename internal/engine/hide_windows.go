package engine

import (
	"os/exec"
	"syscall"
)

// hideWindow stops a console child (ffmpeg) from flashing a window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
