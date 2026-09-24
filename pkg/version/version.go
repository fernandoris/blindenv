// Package version holds build metadata for the BlindEnv binary.
package version

// Version is the semantic version of BlindEnv.
var Version = "0.1.0"

// Commit and Date are injected at build time via -ldflags.
var (
	Commit = "none"
	Date   = "unknown"
)
