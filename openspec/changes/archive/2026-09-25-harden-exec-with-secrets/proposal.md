## Why

`execute_with_secrets` (and `blindenv run`) buffer the entire child stdout/stderr in memory and only truncate to 100 KB *after* the process finishes. Worse, when the child spawns a background process that inherits the pipe, `cmd.Wait` never sees EOF: the 60 s timeout kills only the already-exited direct child, the handler never returns, and memory grows until the operating system kills the process. Reproduced locally: a child emitting ~10 MB/s drove the MCP process to ~1.9 GB, the tool call never returned, and no audit row was written. The same mechanism matches the process termination observed on Windows ("memory pressure"): a bounded command produces an unbounded footprint.

## What Changes

- Bound output capture at the source: stop storing bytes once the cap is reached, instead of buffering everything and truncating afterwards.
- Terminate the whole process tree when the command finishes or times out, so a descendant cannot keep the pipe open (Unix process group; Windows job object or equivalent tree kill).
- Make the execute timeout effective: the handler SHALL return within the deadline even if a descendant still holds stdout/stderr.
- Bound redaction cost: avoid re-copying the whole captured text once per secret and avoid speculative UTF-16 decoding of very large buffers.
- Apply the same bounded capture and process-tree guarantees to `blindenv run`, which today has no timeout at all.
- Add regression tests plus a repeatable benchmark that records the before/after peak memory for the scenarios already reproduced.

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `mcp-server`: `execute_with_secrets` must bound the captured output and must never exceed its timeout or hang when a descendant inherits the child's stdout/stderr.
- `cli`: `run` must bound the captured output and terminate the whole child process tree.
- `secret-redaction`: normalization and substitution must operate within a bounded memory budget and must not amplify memory several times over on large or binary-like input.

## Impact

- Code: `pkg/mcp/exec.go`, `cmd/blindenv/commands.go`, `pkg/mcp/redact.go`, plus small OS-specific helpers for process-group/job-object handling.
- Behavior: the output cap is enforced during capture; a new hard guarantee that execution returns by the timeout; orphans are reaped.
- No MCP tool schema, protocol, storage or CLI signature changes.
