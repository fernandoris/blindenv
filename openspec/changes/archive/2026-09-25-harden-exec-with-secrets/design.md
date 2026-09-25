## Context

See `proposal.md` for motivation and `specs/` for the required behavior.

Current state, grounded in the code:

- `pkg/mcp/exec.go` captures the child with `cmd.Stdout/Stderr = &bytes.Buffer{}` and only truncates in `truncate()` after `cmd.Run()` returns. `maxOutputBytes = 100_000` never bounds capture.
- `cmd/blindenv/commands.go` does the same and, unlike the MCP path, has no timeout at all.
- With `os/exec`, a non-`*os.File` `Stdout` is served by an internal `io.Copy` goroutine, and `Cmd.Wait` waits for that copy to reach EOF. A descendant that inherits the write end prevents EOF, so `Wait` blocks even after the direct child exits.
- `exec.CommandContext` cancels by killing only the direct process; it does not touch descendants.
- `pkg/mcp/redact.go` normalizes the whole captured text, then does one full-copy `strings.ReplaceAll` per key, and speculatively decodes a buffer as UTF-16 when ≥30% of pairs have a zero high byte.

Measured baseline (local, same binary): 200 MB ASCII child output → 727 MB MCP RSS with a returned result; an orphan descendant producing ~10 MB/s → ~1.9 GB RSS, no return, no audit row.

## Goals / Non-Goals

**Goals:**

- Peak memory must be bounded by the retention budget plus a constant, independent of how much the child emits.
- `execute_with_secrets` must return within its timeout even when a descendant holds stdout/stderr.
- The child process tree is terminated on completion and on timeout.
- Redaction remains correct for everything actually returned or emitted.
- Portable: darwin, linux and windows, still `CGO_ENABLED=0`.

**Non-Goals:**

- Changing the MCP tool schemas, the vault, or the audit contract.
- A security sandbox: redaction remains data-loss prevention, not containment.
- Live-streaming the MCP result to the model (the response stays a single bounded payload).
- Replacing the longest-first substitution semantics.

## Decisions

### 1. Bound capture at the source (MCP), stream with redaction (CLI)

The MCP response is a bounded payload for the model, so keep a hard retention budget and enforce it while reading via a custom `io.Writer` that stores up to the budget and then **drains and discards** the rest. Draining is required: a writer that simply stops reading would let the OS pipe fill and block the child. `truncate()` is removed as dead behavior; truncation becomes a property of capture.

`blindenv run` prints to the user's terminal and must not silently eat a large legitimate output, so it changes to a **streaming redacting writer** on stdout/stderr instead of buffer-then-print, keeping only a tail window (max secret length − 1) to catch values split across writes.

Alternatives considered:
- `io.LimitReader` on the child pipe — not applicable to `os/exec` output and would deadlock the child once the pipe fills.
- Keeping `bytes.Buffer` and lowering `maxOutputBytes` — does not bound memory at all; rejected.

### 2. Guarantee Wait returns: process group + `Cancel` + `WaitDelay`

Rely on Go 1.20+ `exec.Cmd` hooks (available: Go 1.27):
- Unix: `SysProcAttr{Setpgid: true}`; `cmd.Cancel` kills the whole group (`syscall.Kill(-pgid, SIGKILL)`).
- Windows: `cmd.Cancel` terminates the tree (Job Object via `golang.org/x/sys/windows`, no CGO; `taskkill /T /F /PID` as fallback).
- `cmd.WaitDelay` set to a short grace (e.g. 2s) so that once the process exits — or the context fires — `Wait` force-closes the I/O pipes and returns even if a descendant still holds them.

This directly fixes the reproduced hang: the orphan case currently blocks `Wait` forever; with `WaitDelay` it returns within the grace and the group kill reaps descendants.

Alternative considered: manual `os.Pipe` plumbing plus `Process.Wait` — more code, more edge cases, unnecessary given `WaitDelay`.

### 3. Redaction over bounded input; fix the UTF-16 heuristic

Because MCP input is now ≤ budget, the per-key `ReplaceAll` cost is negligible and can stay (preserving longest-first correctness). The UTF-16 heuristic is restricted to the budget and only decodes speculatively when the sample is small enough; BOM-detected UTF-16 is always honored. To avoid leaking a fragment when a secret straddles the retention boundary, retain `budget + maxSecretLen` bytes, redact, then trim to the budget.

### 4. Tests and a repeatable benchmark

Add unit tests for the bounded writer, the streaming redactor (split-secret boundary), the timeout/orphan case (a background descendant), and the UTF-16 budget guard. Keep a small benchmark script under the change that re-runs the scenarios from this investigation and prints peak RSS, so the improvement report is reproducible.

## Risks / Trade-offs

- [Secret fragment at the truncation boundary] → retain `budget + maxSecretLen` before redacting, then trim.
- [`WaitDelay` cuts off legitimate late output] → set the grace to a few seconds and kill the group only after the process has exited or the deadline fired.
- [Streaming redaction misses a secret split across writes] → hold back a `maxSecretLen − 1` tail window until the next write or flush.
- [Windows tree-kill portability] → Job Object via `x/sys/windows` (already a dependency) with `taskkill /T /F` fallback; covered by the existing CI matrix.
- [`run` output ordering/exit-code timing changes] → output may now appear before exit; exit-code semantics are unchanged.

## Migration Plan

No data migration. Change ships in the binary; configure nothing. Rollback is reverting the commit. Document the new guarantee in the README/`How it works`.

## Open Questions

- Whether to expose the retention budget as an env var or tool argument (e.g. `BLINDENV_MAX_OUTPUT_BYTES`). Default stays the current 100 KB; deferrable without changing the specs.
- Exact `WaitDelay` grace value; tune during implementation.
