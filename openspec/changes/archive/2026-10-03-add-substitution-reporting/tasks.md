## 1. Substitution helpers

- [x] 1.1 Change `substitute` in `pkg/mcp/proxy.go` to return `(string, int)` counting `{{KEY}}` occurrences, and update `substituteBody`/`substituteJSON` to thread the count; verify with a table test covering repeated tags and no-tag input
- [x] 1.2 Add a shared helper that scans caller-supplied text for `{{...}}` tags and returns the distinct names not present in the effective key set, deduplicated and capped at 20; verify with a test for dedup, cap and empty input
- [x] 1.3 Change `translateSecretTags` in `pkg/mcp/shell.go` to also return the rewritten-tag count and unmatched tag names folded into its existing scan; verify existing `shell_test.go` cases still pass and the new return values match expectations
- [x] 1.4 Clarify the `StreamRedactor.Count` doc comment in `pkg/mcp/capture.go` to say redactions; verify `go vet ./...` is clean

## 2. Tool result wiring

- [x] 2.1 Add `substitutions` and `unmatched_tags` to `proxyResult`, populate them from the helpers, and verify `TestProxySubstitutesAndRedacts` plus a new count assertion pass
- [x] 2.2 Add `substitutions` and `unmatched_tags` to `execResult` across the success and translated paths, and verify `exec_test.go` count assertions pass
- [x] 2.3 Add `substitutions` and `unmatched_tags` to `openResult` (no `redactions`), and verify `open_test.go` count assertions pass
- [x] 2.4 Add a test per tool proving `substitutions > 0` with `redactions == 0` when the response omits the value, and `substitutions == 0` with a listed unmatched tag for a wrong key name

## 3. Audit persistence

- [x] 3.1 Add `Substitutions int` to `db.AuditEntry`, the `substitutions` column to `baseSchemaSQL`, and extend the audit insert/select in `pkg/db/store.go`; verify `db_test.go` round-trips the value
- [x] 3.2 Add the guarded additive migration `migrateAuditSubstitutions` and bump `schemaVersion` to 5; verify opening a v4 database adds the column with default 0 and preserves rows
- [x] 3.3 Thread the substitution count through `Server.audit` (`pkg/mcp/server.go`) and every call site; verify refusal paths still record 0
- [x] 3.4 Expose `substitutions` in the audit view in `pkg/web/server.go`; verify the web JSON includes it

## 4. Performance and memory verification

- [x] 4.1 Add `BenchmarkSubstituteCount` (with and without matches) in `pkg/mcp/perf_test.go` and run `go test -run=^$ -bench=Substitute -benchmem ./pkg/mcp`; confirm allocations stay flat across input sizes
- [x] 4.2 Run the full suite `go test ./...` after the change and confirm no regressions in `pkg/mcp` and `pkg/db` performance/memory tests

## 5. Documentation

- [x] 5.1 Update `README.md` to describe `substitutions` and `unmatched_tags` and the distinction from `redactions`; verify the description matches the tool result shapes in the spec
- [x] 5.2 Run `openspec validate add-substitution-reporting --strict` and confirm it passes
