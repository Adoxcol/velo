// Package version holds Velo's version, set at build time:
//
//	go build -ldflags "-X velo/internal/version.Version=0.1.0" ...
package version

// Version is "dev" for local builds.
var Version = "dev"
