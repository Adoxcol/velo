package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// hideWindow stops a console child (ffmpeg) from flashing a window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

func killProcessTree(cmd *exec.Cmd) error {
	kill := exec.Command("taskkill.exe", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
	hideWindow(kill)
	if err := kill.Run(); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}

// findPlatformTool locates toolName (e.g. "ffmpeg.exe") when the current
// process's PATH is stale (tool installed while Velo was already running) or
// truncated by Windows (when System + User PATH exceeds 4,095 characters and
// Explorer drops HKCU\Environment\Path).
func findPlatformTool(toolName string) string {
	for _, root := range []struct {
		key  registry.Key
		path string
	}{
		{registry.CURRENT_USER, `Environment`},
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`},
	} {
		if p := searchPathList(toolName, readRegistryPath(root.key, root.path)); p != "" {
			return p
		}
	}

	localAppData := os.Getenv("LOCALAPPDATA")
	userProfile := os.Getenv("USERPROFILE")
	progData := os.Getenv("ProgramData")
	var dirs, globs []string
	if localAppData != "" {
		dirs = append(dirs,
			filepath.Join(localAppData, "Programs", "Velo"),
			filepath.Join(localAppData, "Microsoft", "WinGet", "Links"),
		)
		globs = append(globs,
			filepath.Join(localAppData, "Microsoft", "WinGet", "Packages", "*", "*", "bin", toolName),
			filepath.Join(localAppData, "Microsoft", "WinGet", "Packages", "*", toolName),
		)
	}
	if userProfile != "" {
		dirs = append(dirs,
			filepath.Join(userProfile, "scoop", "shims"),
			filepath.Join(userProfile, ".deno", "bin"),
			filepath.Join(userProfile, ".local", "bin"),
		)
		globs = append(globs,
			filepath.Join(userProfile, "scoop", "apps", "*", "current", "bin", toolName),
			filepath.Join(userProfile, "scoop", "apps", "*", "current", toolName),
		)
	}
	if progData != "" {
		dirs = append(dirs, filepath.Join(progData, "chocolatey", "bin"))
	}
	dirs = append(dirs, `C:\ffmpeg\bin`)

	for _, dir := range dirs {
		if p := filepath.Join(dir, toolName); isRegularFile(p) {
			return p
		}
	}
	for _, pattern := range globs {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			if isRegularFile(m) {
				return m
			}
		}
	}
	return ""
}

func readRegistryPath(root registry.Key, subkey string) string {
	k, err := registry.OpenKey(root, subkey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("Path")
	if err != nil {
		return ""
	}
	return v
}

func searchPathList(toolName, pathList string) string {
	for _, raw := range strings.Split(pathList, ";") {
		dir := strings.Trim(strings.TrimSpace(expandWinEnv(raw)), `"`)
		if dir == "" {
			continue
		}
		if p := filepath.Join(dir, toolName); isRegularFile(p) {
			return p
		}
	}
	return ""
}

// expandWinEnv expands %VAR% placeholders in REG_EXPAND_SZ registry values.
func expandWinEnv(s string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		name := s[i+1 : i+1+j]
		if val := os.Getenv(name); val != "" {
			b.WriteString(val)
		} else {
			b.WriteString(s[i : i+2+j])
		}
		s = s[i+2+j:]
	}
}

