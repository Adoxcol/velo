//go:build !windows

package update

import "os/exec"

var runInstaller = defaultRunInstaller

func defaultRunInstaller(path string) error {
	cmd := exec.Command(path, "/S", "/update")
	return cmd.Start()
}
