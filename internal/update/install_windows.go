//go:build windows

package update

import (
	"os/exec"
	"syscall"
)

var execCommand = exec.Command

func defaultRunInstaller(path string) error {
	cmd := execCommand(path, "/S", "/update")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
