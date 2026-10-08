## Why

`blindenv version` reports `commit none, built unknown` for every build: nothing injects `version.Commit` / `version.Date`, and the command ignores the build metadata Go already embeds (`runtime/debug.ReadBuildInfo`). As a result two binaries from very different commits are indistinguishable at runtime. This caused a real incident: a stale build on `PATH` silently opened a newer vault, failed to decrypt non-sensitive configuration values (its error looked like a plaintext corruption bug), and regressed the persisted schema marker before the current binary re-migrated it. Builds must identify themselves so a stale binary is obvious immediately.

## What Changes

- `blindenv version` SHALL report a real build identity: the semantic version plus the VCS revision, commit time and a "modified" indicator, or the module version for a `go install <module>@<version>` build.
- Resolution precedence: `-ldflags` injection wins, then Go's embedded build info, then the current defaults. The command MUST NOT print `none` / `unknown` when the Go build info carries the information.
- Add stable build tooling: `make build` injects version, commit and date via `-ldflags`; `make install` builds and copies the binary to a stable location so the binary the `PATH` resolves can be kept current.
- Document how to build, how to verify which binary is running, and how to update the `PATH` binary.

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `cli`: the `version` command must report the real build identity (VCS revision, commit time, modified flag, or module version) derived from build info when `-ldflags` are absent, instead of unconditional `none` / `unknown` placeholders.

## Impact

- `pkg/version/version.go` — resolve effective build metadata from ldflags, then `runtime/debug.ReadBuildInfo`, then defaults.
- `cmd/blindenv/main.go` — `printVersion` renders the resolved identity.
- Tests: a `version` unit test asserting ldflags precedence and build-info fallback; a CLI test asserting `blindenv version` never prints the placeholders for an instrumented build.
- Tooling/docs: `Makefile` (`build`, `install`), `README.md` / `CONTRIBUTING.md`.
- No changes to the vault format, crypto, backup format, MCP contract or web UI.
