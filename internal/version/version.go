// Package version carries build metadata injected at link time.
package version

// These values are overridden by -ldflags in the release build.
var (
	Version = "1.3.0"
	Commit  = "dev"
	Date    = "unknown"
)
