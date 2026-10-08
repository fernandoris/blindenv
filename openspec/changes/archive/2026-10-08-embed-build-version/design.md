## Context

See proposal.md - Why. Today `pkg/version` holds a hard-coded `Version = "0.1.0"` and `Commit` / `Date` variables that are documented as "injected at build time via -ldflags", but nothing injects them: the `Makefile` has no build target, CI runs a plain `go build ./...`, and `go install` passes no flags. `printVersion` (`cmd/blindenv/main.go:94`) therefore always prints `commit none, built unknown`.

Go already embeds an identity in every binary and exposes it at runtime through `runtime/debug.ReadBuildInfo`:

- Built from a VCS checkout: `bi.Settings` carries `vcs.revision`, `vcs.time`, `vcs.modified`.
- Built from the module cache (`go install <module>@<version>`): `bi.Main.Version` carries the module version (a pseudo-version when there is no tag); VCS settings are absent.

`-ldflags -X` can still override everything, and is the only way to pin a date/version when building outside a checkout.

## Goals / Non-Goals

**Goals:**
- `blindenv version` always reports a usable build identity, derived from ldflags when present and from Go build info otherwise.
- The `PATH` binary can be rebuilt and replaced deterministically via a documented `make build` / `make install`.

**Non-Goals:**
- No change to the vault format, crypto, backup format, MCP contract or web UI.
- Not a runtime version-stamp inside the vault; build identity is a property of the binary.
- No new external dependency.

## Decisions

### D1. Resolve the effective identity in `pkg/version`

Add a resolver that returns the effective (version, commit, date, modified) with fixed precedence, keeping the existing exported variables as the ldflags targets:

1. `Commit` / `Date` when set to something other than the `none` / `unknown` defaults.
2. `buildinfo` `vcs.revision` / `vcs.time` / `vcs.modified`.
3. `bi.Main.Version` when it is not `(devel)`, used as the commit identity for module installs.
4. The current defaults.

- Why: this fixes the common cases with no build-time cooperation while preserving `-ldflags` as the escape hatch for reproducible/release builds.
- Alternative considered: derive everything from `git` at build time in the `Makefile`. Rejected as the sole mechanism - `go install ...@latest` and plain `go build` bypass the `Makefile`, which is exactly how the stale binary arose.

### D2. Keep the output one line; append a modified marker

`printVersion` keeps its single-line shape and prints the resolved commit and date. When the working tree was modified, the commit gains a `-dirty`-style suffix (or an explicit `modified` token) so a locally patched build is visible.

- Why: preserves the existing human-readable contract from the `cli` spec and makes the "same 0.1.0, different binary" case obvious.

### D3. Build tooling with a stable install path

Add `Makefile` targets:

- `build`: `CGO_ENABLED=0 go build -ldflags "-X .../pkg/version.Version=$(VERSION) -X .../pkg/version.Commit=$(COMMIT) -X .../pkg/version.Date=$(DATE)" -o bin/blindenv ./cmd/blindenv`, with `COMMIT`/`DATE` from `git` and safe fallbacks.
- `install`: depends on `build`, then installs to `$(PREFIX)/bin/blindenv` (default `PREFIX ?= /usr/local`), so a machine can run `make install PREFIX=/opt/homebrew`.

- Why: gives one command that both stamps the build and places it where the `PATH` resolves, closing the loop that `go install` leaves open (the Go bin directory is not on this developer's `PATH`).
- Alternative considered: a shell script instead of `Makefile` targets. Rejected - the repo already uses `make` for `fmt`/`check`.

### D4. Tests target the resolver, not the environment

Unit-test the precedence logic by feeding synthetic `buildinfo` values to the resolver, and keep a CLI-level test asserting `blindenv version` prints zero for the placeholder tokens when an identity is present. Do not assert on the developer's actual `git` state.

## Risks / Trade-offs

- [`go install ...@version` builds have no VCS info] -> Fall back to `bi.Main.Version`; the pseudo-version still identifies the commit.
- [`-ldflags` path format is easy to typo] -> Centralize it in one `Makefile` variable and assert the injected value in a test.
- [`vcs.modified` is only meaningful inside a checkout] -> Treat absence as "not modified"; never fabricate it.
- [Output format change could break a script parsing `blindenv version`] -> Low risk; no known parsers. Keep the leading `blindenv <version>` token stable.

## Migration Plan

No data or vault migration. Operational only: rebuild with `make build` (or `make install`), replace the binary the `PATH` resolves, and confirm with `blindenv version` that a real revision is shown. Rollback is restoring the previous binary.

## Open Questions

None.
