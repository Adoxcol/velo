// Package velo provides the release notes bundled with the application.
package velo

import (
	_ "embed"
	"strings"
)

//go:embed CHANGELOG.md
var changelog string

// ReleaseNotes returns the changelog section for an exact release version.
func ReleaseNotes(version string) string {
	var lines []string
	active := false
	for _, line := range strings.Split(changelog, "\n") {
		if strings.HasPrefix(line, "## ") {
			if active {
				break
			}
			fields := strings.Fields(line)
			active = len(fields) > 1 && fields[1] == version
			continue
		}
		if active {
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
