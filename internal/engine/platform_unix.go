//go:build linux || darwin

package engine

import (
	"golang.org/x/sys/unix"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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

// findPlatformTool checks standard Unix package-manager and user bin locations
// when a GUI/browser host starts with a minimal PATH.
func findPlatformTool(toolName string) string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, ".deno", "bin"),
		)
	}
	dirs = append(dirs,
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/home/linuxbrew/.linuxbrew/bin",
		"/snap/bin",
	)
	for _, dir := range dirs {
		if p := filepath.Join(dir, toolName); isRegularFile(p) {
			return p
		}
	}
	return ""
}

