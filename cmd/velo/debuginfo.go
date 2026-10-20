//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/Adoxcol/velo/internal/version"
)

// Link query strings and fragments often hold login tokens; bug reports
// keep only scheme, host and path.
var urlSecrets = regexp.MustCompile(`(https?://[^\s?#"]+)[?#][^\s"]*`)

// debugInfo is a plain-text report for bug reports: versions, settings and
// the end of the log, with link query strings removed.
func (a *app) debugInfo() string {
	var b strings.Builder
	maj, min, build := windows.RtlGetNtVersionNumbers()
	fmt.Fprintf(&b, "Velo %s (%s/%s, %s)\nWindows %d.%d.%d\n", version.Version, runtime.GOOS, runtime.GOARCH, runtime.Version(), maj, min, build)
	ext := extensionVersion()
	if ext == "" {
		ext = "not installed next to Velo"
	}
	seen := "never"
	if t := a.extensionSeen(); t > 0 {
		seen = time.Since(time.UnixMilli(t)).Round(time.Second).String() + " ago"
	}
	fmt.Fprintf(&b, "Extension: %s, last heard from %s\n", ext, seen)
	a.mu.Lock()
	n, running := len(a.items), a.running
	a.mu.Unlock()
	fmt.Fprintf(&b, "Downloads: %d in list, %d running\n", n, running)

	s := a.getSettings()
	if s.DownloadDir != "" {
		s.DownloadDir = filepath.Base(s.DownloadDir) // the folder name, not the user's path
	}
	sj, _ := json.Marshal(s)
	fmt.Fprintf(&b, "Settings: %s\n", sj)

	b.WriteString("\nRecent log:\n")
	b.WriteString(urlSecrets.ReplaceAllString(tailLog(filepath.Join(a.home, "velo.log"), 80), "$1?…"))
	return b.String()
}

// tailLog returns the last n lines of the log.
func tailLog(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "(no log: " + err.Error() + ")\n"
	}
	if len(data) > 256<<10 {
		data = data[len(data)-256<<10:]
	}
	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	home, _ := os.UserHomeDir()
	out := string(bytes.Join(lines, []byte("\n"))) + "\n"
	if home != "" { // C:\Users\<name> → %USERPROFILE%
		out = strings.ReplaceAll(out, home, `%USERPROFILE%`)
	}
	return out
}
