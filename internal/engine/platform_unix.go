//go:build linux || darwin

package engine

import (
	"golang.org/x/sys/unix"
	"net/url"
	"os/exec"
	"syscall"
)

// On Unix, proxyFor uses HTTP_PROXY, HTTPS_PROXY and NO_PROXY.
func systemProxy(*url.URL) *url.URL { return nil }

var freeSpace = func(dir string) int64 {
	var stat unix.Statfs_t
	if err := unix.Statfs(dir, &stat); err != nil {
		return -1
	}
	return int64(stat.Bavail) * int64(stat.Bsize)
}

// A separate group lets cancellation stop yt-dlp and its ffmpeg children.
func hideWindow(cmd *exec.Cmd)            { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func killProcessTree(cmd *exec.Cmd) error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
