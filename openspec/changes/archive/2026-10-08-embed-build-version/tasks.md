## 1. Build identity resolver

- [x] 1.1 Add an effective-identity resolver in `pkg/version` that reads `runtime/debug.ReadBuildInfo` and applies the precedence ldflags > `vcs.revision`/`vcs.time`/`vcs.modified` > `bi.Main.Version` > defaults; verify with table-driven unit tests that feed synthetic build info and assert each precedence level.
- [x] 1.2 Model the working-tree modified state so the resolver reports a dirty marker when `vcs.modified` is true and no marker when it is false or absent; verify a unit test covers both cases.
- [x] 1.3 Keep `Version`, `Commit` and `Date` as the `-ldflags` targets and preserve the existing package exports; verify `CGO_ENABLED=0 go build ./...` still compiles every reference unchanged.

## 2. CLI output

- [x] 2.1 Update `printVersion` to render the resolved identity (version, commit with dirty marker, date, go/os/arch/compiler) on a single line; verify a CLI test runs `blindenv version` for an instrumented build and asserts a real revision is shown and the `commit none` / `built unknown` placeholders are absent.
- [x] 2.2 Confirm the command still writes only to stdout and exits zero; verify the same test asserts exit code 0 and empty stderr.

## 3. Build tooling

- [x] 3.1 Add a `build` target that stamps `Version`/`Commit`/`Date` through `-ldflags` from `git` with safe fallbacks and writes `bin/blindenv`; verify `make build` produces a binary whose `blindenv version` reports the current commit.
- [x] 3.2 Add an `install` target with `PREFIX ?= /usr/local` that installs the built binary to `$(PREFIX)/bin`; verify `make install PREFIX=$(mktemp -d)` places an executable there and running it prints the build identity.
- [x] 3.3 Ignore the build output directory (add `bin/` to `.gitignore` if absent); verify `git status` stays clean after `make build`.

## 4. Documentation

- [x] 4.1 Document `make build` / `make install` and how to verify which binary is running (`blindenv version`, `go version -m`) in `README.md` and the build section of `CONTRIBUTING.md`; verify the documented commands match the `Makefile` targets.
- [x] 4.2 Document the stale-binary failure mode that motivates identifying builds, so an operator knows to check `blindenv version` when secret tools fail; verify by review.

## 5. Verification

- [x] 5.1 Run `make check`; verify gofmt, `go vet`, build and the full test suite pass.
- [x] 5.2 End-to-end: create a fresh vault containing one non-sensitive configuration value and one sensitive secret, then run `execute_with_secrets` with the installed binary; verify the configuration value is injected unredacted, the secret is redacted, and no decrypt error occurs.
