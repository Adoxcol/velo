//go:build windows

package update

import (
	"os/exec"
	"syscall"
)

func runInstaller(path string) error {
	cmd := exec.Command(path, "/S", "/update")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Start()
}
