//go:build !windows

package update

import "os/exec"

func runInstaller(path string) error {
	cmd := exec.Command(path, "/S", "/update")
	return cmd.Start()
}
