## 1. Capture primitives

- [x] 1.1 Add a bounded `io.Writer` that retains up to `maxOutputBytes` and drains the remainder without storing it; verify with a unit test feeding 10x the budget that retained length equals the budget and total bytes are drained.
- [x] 1.2 Add a streaming redacting writer that holds back a `maxSecretLen-1` tail window and flushes redacted output on close; verify with a unit test where a secret value is split across two writes and the emitted stream contains no fragment of the value.
- [x] 1.3 Confirm `NewRedactor`/`Redact` still satisfy longest-first and minimum-length behavior after the input is bounded; verify `go test ./pkg/mcp/...` passes.

## 2. Process-tree termination and effective timeout

- [x] 2.1 Add a Unix process-group launcher (`Setpgid`) and a group-kill `Cancel` function; verify with a test that a child which spawns a background descendant is fully reaped after cancellation.
- [x] 2.2 Add the Windows tree-kill path (Job Object via `x/sys/windows`, `taskkill /T /F` fallback); verify `CGO_ENABLED=0 GOOS=windows go build ./...` succeeds.
- [x] 2.3 Set `cmd.WaitDelay` so `Wait` returns after the process exits or the deadline fires even when a descendant holds stdout/stderr; verify with a test that a descendant holding the pipe does not block beyond the grace.
- [x] 2.4 Make `handleExecute` return a timeout error from the deadline path and ensure the process tree is terminated; verify with a test that a long-running command returns within the timeout and does not leave descendants.

## 3. Wire into execute_with_secrets

- [x] 3.1 Replace the `bytes.Buffer` capture in `pkg/mcp/exec.go` with the bounded writer and remove post-hoc `truncate`; verify a test that 200 MB of child output keeps peak allocation bounded and returns a truncated result with the exit code.
- [x] 3.2 Retain `budget + maxSecretLen` bytes before redaction and trim afterward so a secret straddling the boundary cannot leak a fragment; verify with a test placing a secret at the boundary and asserting no fragment is returned.
- [x] 3.3 Verify an audit entry is written with exit code and redaction count on both the normal and truncated paths; assert via `ListAudit` in a test.

## 4. Wire into blindenv run

- [x] 4.1 Replace the `bytes.Buffer` capture in `cmd/blindenv/commands.go` with streaming redacting writers on stdout/stderr; verify a test that large output is emitted with secrets redacted and memory stays flat.
- [x] 4.2 Apply the same process-group/tree termination so `run` cannot hang on a descendant; verify a test where the child spawns a background descendant and `run` still returns and propagates the exit code.

## 5. Encoding normalization budget

- [x] 5.1 Restrict speculative BOM-less UTF-16 detection to the bounded input and keep BOM-detected UTF-16 always honored; verify existing `TestRedactorUTF16LE` and a new test that a large non-UTF-16 buffer is not decoded as UTF-16.
- [x] 5.2 Verify the normalization+substitution path over the retained prefix stays within a bounded allocation with a benchmark or `testing.AllocsPerRun` check.

## 6. Regression coverage

- [x] 6.1 Add an MCP-level test driving `execute_with_secrets` over the tool handler for the large-output and descendant-holding cases; verify the handler returns within the deadline.
- [x] 6.2 Run `go test ./...`, `go vet ./...` and `CGO_ENABLED=0 go build ./...`; verify all pass.

## 7. Improvement report

- [x] 7.1 Re-run the six reproduced scenarios (run: 50 MB, 200 MB, 50 MB NUL, 300 MB orphan; MCP: 200 MB, slow orphan) against the fixed binary and record peak RSS and whether the tool returned.
- [x] 7.2 Produce a before/after table in the change directory comparing baseline peak RSS and tool-return behavior with the post-fix numbers, and state the improvement for the orphan case.
