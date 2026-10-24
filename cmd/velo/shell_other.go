//go:build !windows

package main

import (
	"os/exec"
	"path/filepath"
)

func openInFolder(targetPath string) error {
	dir := filepath.Dir(targetPath)
	return exec.Command("xdg-open", dir).Start()
}

func openPath(targetPath string) error {
	return exec.Command("xdg-open", targetPath).Start()
}
