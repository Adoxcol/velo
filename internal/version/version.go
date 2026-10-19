// Package version holds Velo's version, set at build time:
//
//	go build -ldflags "-X github.com/Adoxcol/velo/internal/version.Version=0.1.0" ...
package version

import (
	"runtime/debug"
	"strings"
)

// Version is "dev" for local builds.
var Version = "dev"

// Display is Version, or for "go install …@v0.8.3" builds (which carry no
// -ldflags) the module version Go recorded, so `veloctl version` is useful.
func Display() string {
	if Version != "dev" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return Version
}
