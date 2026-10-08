// Package version holds build metadata for the BlindEnv binary.
package version

import (
	"runtime/debug"
)

// Version is the semantic version of BlindEnv.
var Version = "0.1.0"

// Commit and Date are injected at build time via -ldflags.
var (
	Commit = "none"
	Date   = "unknown"
)

// Identity placeholders printed when no build information is available.
const (
	placeholderCommit = "none"
	placeholderDate   = "unknown"
)

// Build is the effective build identity of the binary.
type Build struct {
	// Version is the semantic version.
	Version string
	// Commit is the source revision, or the module version for a
	// `go install <module>@<version>` build.
	Commit string
	// Date is the commit time, or the injected build date.
	Date string
	// Modified reports whether the working tree had uncommitted changes when
	// the binary was built from a version-control checkout.
	Modified bool
}

// Resolve returns the effective build identity of the running binary. Values
// injected at build time through -ldflags take precedence over the identity the
// Go toolchain embeds, and either source takes precedence over the placeholders.
func Resolve() Build {
	bi, _ := debug.ReadBuildInfo()
	return resolve(Version, Commit, Date, bi)
}

// resolve is the testable core of Resolve. It applies the precedence
// ldflags > vcs.revision / vcs.time / vcs.modified > module version > defaults.
func resolve(version, commit, date string, bi *debug.BuildInfo) Build {
	b := Build{Version: version, Commit: commit, Date: date}
	if bi == nil {
		return b
	}
	settings := make(map[string]string, len(bi.Settings))
	for _, s := range bi.Settings {
		settings[s.Key] = s.Value
	}
	if b.Commit == placeholderCommit {
		if rev := settings["vcs.revision"]; rev != "" {
			b.Commit = rev
		} else if mv := bi.Main.Version; mv != "" && mv != "(devel)" {
			b.Commit = mv
		}
	}
	if b.Date == placeholderDate {
		if t := settings["vcs.time"]; t != "" {
			b.Date = t
		}
	}
	b.Modified = settings["vcs.modified"] == "true"
	return b
}
