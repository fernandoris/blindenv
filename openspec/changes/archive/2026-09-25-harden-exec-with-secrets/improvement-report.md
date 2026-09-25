# Improvement report: harden-exec-with-secrets

Before/after measurement of the memory behavior fixed by this change.

## Method

- Same machine (macOS arm64, Go 1.27.1), same scenarios, same output volumes.
- Baseline binary: original `cmd/blindenv` build (`bytes.Buffer` capture, no
  process-tree kill). Fixed binary: same source with this change applied.
- Metric: `/usr/bin/time -l` **peak memory footprint**, except the baseline MCP
  orphan, which never returned and was sampled with `ps` (max RSS) while it grew.
- `run` scenarios drive `blindenv run /x -- ...` (no project needed).
- MCP scenarios drive `execute_with_secrets` over the real stdio JSON-RPC
  interface against a `lab` project with `allow_execute = 1`.

## Results

```
Scenario                         Baseline    Fixed     Reduction   Returned after fix
------------------------------------------------------------------------------
run   50 MB ASCII                 246 MB     129 MB      1.9x       n/a
run   200 MB ASCII                950 MB     146 MB      6.5x       n/a
run   50 MB NUL (UTF-16 hint)     427 MB     135 MB      3.2x       n/a
run   300 MB orphan              1688 MB     147 MB     11.5x       n/a
MCP   200 MB ASCII                751 MB      82 MB      9.2x       yes
MCP   slow orphan               ~1938 MB*     83 MB    ~23x        yes (was: never)
```

\* baseline MCP orphan peak sampled with `ps` max RSS; the tool call never
returned and the process grew until externally killed.

## Notes

- Fixed `run` and MCP peaks are **independent of output volume**: 50 MB and
  200 MB `run` outputs land at 129 MB and 146 MB, and the 300 MB orphan settles
  at 147 MB. The residual is the fixed startup cost (Argon2id ~64 MB + Go
  runtime/SQLite), not growth with the child's output.
- The MCP orphan is the decisive fix: the 60 s timeout is now effective — the
  handler returned with `exit_code: 0` and a truncated 100 KB stdout, and an
  audit row was written. Before, no response and no audit row were ever
  produced.
- MCP responses now report `exit_code: 0` with `stdout` capped at the 100 000
  byte budget for both scenarios, verified from the tool results.
- Both MCP scenarios produced one audit entry each, with exit code recorded.

## Verification commands

- Unit/regression tests: `go test ./...` (all packages pass).
- Static checks: `go vet ./...`, `CGO_ENABLED=0 go build ./...`,
  `CGO_ENABLED=0 GOOS=windows go build ./...`.
- New tests cover: bounded writer, split-secret streaming redaction, streaming
  UTF-16 normalization, large non-UTF-16 not decoded, allocation bound,
  large-output truncation + audit, secret at the budget boundary, timeout
  return, descendant-holding-pipe return, descendant reaping, and `run`
  streaming and background-descendant return.
