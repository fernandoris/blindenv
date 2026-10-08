package version

import (
	"runtime/debug"
	"testing"
)

func TestResolvePrecedence(t *testing.T) {
	vcsDirty := &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
			{Key: "vcs.time", Value: "2026-10-06T17:27:48Z"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	vcsClean := &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef01234567"},
			{Key: "vcs.time", Value: "2026-10-06T17:27:48Z"},
			{Key: "vcs.modified", Value: "false"},
		},
	}
	moduleInstall := &debug.BuildInfo{
		Main: debug.Module{Version: "v0.0.0-20261006172748-2e50a8a3295a"},
	}

	cases := []struct {
		name    string
		version string
		commit  string
		date    string
		bi      *debug.BuildInfo
		want    Build
	}{
		{
			name: "defaults without build info", version: "0.1.0", commit: "none", date: "unknown",
			bi:   nil,
			want: Build{Version: "0.1.0", Commit: "none", Date: "unknown"},
		},
		{
			name: "vcs revision and date, dirty tree", version: "0.1.0", commit: "none", date: "unknown",
			bi: vcsDirty,
			want: Build{
				Version:  "0.1.0",
				Commit:   "0123456789abcdef0123456789abcdef01234567",
				Date:     "2026-10-06T17:27:48Z",
				Modified: true,
			},
		},
		{
			name: "vcs revision and date, clean tree", version: "0.1.0", commit: "none", date: "unknown",
			bi: vcsClean,
			want: Build{
				Version:  "0.1.0",
				Commit:   "0123456789abcdef0123456789abcdef01234567",
				Date:     "2026-10-06T17:27:48Z",
				Modified: false,
			},
		},
		{
			name: "module version fallback", version: "0.1.0", commit: "none", date: "unknown",
			bi: moduleInstall,
			want: Build{
				Version: "0.1.0",
				Commit:  "v0.0.0-20261006172748-2e50a8a3295a",
				Date:    "unknown",
			},
		},
		{
			name: "ldflags take precedence over build info", version: "0.2.0", commit: "abc1234", date: "2026-01-02T00:00:00Z",
			bi: vcsDirty,
			want: Build{
				Version:  "0.2.0",
				Commit:   "abc1234",
				Date:     "2026-01-02T00:00:00Z",
				Modified: true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolve(tc.version, tc.commit, tc.date, tc.bi); got != tc.want {
				t.Fatalf("resolve() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveModifiedState(t *testing.T) {
	clean := resolve("0.1.0", "none", "unknown", &debug.BuildInfo{
		Main:     debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.modified", Value: "false"}},
	})
	if clean.Modified {
		t.Fatalf("modified = true for a clean tree: %+v", clean)
	}
	absent := resolve("0.1.0", "none", "unknown", &debug.BuildInfo{
		Main:     debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}},
	})
	if absent.Modified {
		t.Fatalf("modified = true when vcs.modified is absent: %+v", absent)
	}
	dirty := resolve("0.1.0", "none", "unknown", &debug.BuildInfo{
		Main:     debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc"}, {Key: "vcs.modified", Value: "true"}},
	})
	if !dirty.Modified {
		t.Fatalf("modified = false for a dirty tree: %+v", dirty)
	}
}

func TestResolveRunningBinary(t *testing.T) {
	// Resolve must never panic and must report the semantic version.
	if got := Resolve().Version; got != Version {
		t.Fatalf("Resolve().Version = %q, want %q", got, Version)
	}
}
