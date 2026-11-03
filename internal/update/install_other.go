//go:build !windows

package update

import "os/exec"

var execCommand = exec.Command

func defaultRunInstaller(path string) error {
	cmd := execCommand(path, "/S", "/update")
	return cmd.Start()
}
