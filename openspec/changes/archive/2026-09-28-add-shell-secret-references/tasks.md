## 1. Shell classification and reference mapping

- [x] 1.1 Add a helper that classifies a `shell` argument (powershell/pwsh/cmd/POSIX/unknown/empty) and returns the native environment reference for a key name using brace forms (`${env:NAME}`, `%NAME%`, `${NAME}`). Verify with a table-driven test in `pkg/mcp` covering every recognized shell, unknown and empty.
- [x] 1.2 Verify brace forms tolerate non-identifier key names (e.g. `MY-KEY`) in the reference output (test).

## 2. Quote-aware tag translation

- [x] 2.1 Implement a translation that scans a command string for `{{NAME}}` tags, rewrites only those whose name is an effective key to the shell reference, and leaves every other tag unchanged. Verify with tests for a matching tag, a non-matching tag, and multiple tags.
- [x] 2.2 Track single/double quote state per shell and refuse with an actionable error when a matching tag is inside a single-quoted region. Verify with tests for POSIX and PowerShell; verify `cmd` has no single-quote restriction.
- [x] 2.3 Refuse with an error when a matching tag is present and the shell is empty or unknown. Verify with tests asserting no translation occurs and the error is returned.
- [x] 2.4 Verify an empty command or a command with no tags is returned unchanged with no error (test).

## 3. Wire translation into execute_with_secrets

- [x] 3.1 In `handleExecute` (`pkg/mcp/exec.go`) translate the command before building the shell invocation; on translation error return a tool error without starting a process. Verify a test that a matching tag yields the native reference in the child, and that the value never appears in the invoked command line.
- [x] 3.2 Verify the translated command still redacts child output and still injects the secret into the environment (extend an existing exec test).
- [x] 3.3 Verify a command with a non-matching tag still runs and is passed through (test).

## 4. get_context secret reference

- [x] 4.1 Add an optional `shell` argument to the `get_context` tool and return a `secret_reference` object (`shell`, `native`, `tag`) for the resolved shell, defaulting to the OS shell. Verify with tests for an explicit PowerShell shell, an explicit cmd shell, and the default.
- [x] 4.2 Verify `get_context` still never returns values and its other fields are unchanged (existing and new tests).

## 5. Instructions and documentation

- [x] 5.1 Update the server `instructions` (`pkg/mcp/server.go`) to describe reading the shell's secret reference from `get_context` between discovery and use; verify a test asserts the reference step is mentioned.
- [x] 5.2 Update the README to document that `{{SECRET_NAME}}` works in `proxy_http_request` and `execute_with_secrets`, that commands use the shell-native reference, and that a matching tag inside single quotes or with an unknown shell is refused.

## 6. End-to-end verification

- [x] 6.1 Run `go test ./...` and `go vet ./...` and confirm they pass.
- [x] 6.2 Run `openspec validate add-shell-secret-references` and confirm the change is valid.

## 7. Performance

- [x] 7.1 Add `go test -bench` benchmarks for the tag translation with `b.ReportAllocs()`: a command with one matching tag, a command with many matching tags, and a long command with many effective keys but no tags. Verify the results print ns/op and allocs/op.
- [x] 7.2 Verify with `testing.AllocsPerRun` that translating a command with no tags does not allocate per effective key, guarding against an O(keys x length) regression. Verify the assertion fails if the translation loops over every key with `strings.ReplaceAll`.
- [x] 7.3 Verify scaling is linear in command length and tag count (benchmark across two input sizes), and record the numbers in the change directory as a short performance note.
- [x] 7.4 Measure the `execute_with_secrets` overhead added by translation (benchmark the handler path or the translation step on a representative command) and confirm it is negligible relative to process startup; record the result in the performance note.
