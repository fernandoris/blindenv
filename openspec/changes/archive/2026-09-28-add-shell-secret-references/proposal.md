## Why

`{{SECRET_NAME}}` is only meaningful in `proxy_http_request`, where BlindEnv substitutes the value in Go. Inside `execute_with_secrets`, BlindEnv performs no substitution and injects secrets as environment variables, so a `{{SECRET_NAME}}` tag reaches the shell verbatim. On Windows, PowerShell tokenizes `{` / `}` as script-block delimiters (and `@{` as a hashtable), so the tag is parsed as code instead of a value — the substitution the agent expected never happens and the command can fail to parse. The agent now has to know a different reference syntax per shell, which is exactly the kind of context-specific detail that keeps breaking.

## What Changes

- Keep `{{SECRET_NAME}}` as the single, portable tag. In `proxy_http_request` it continues to substitute the value (in Go, no shell); in `execute_with_secrets` BlindEnv translates it to the **shell-native environment reference** (`${env:NAME}` for PowerShell/pwsh, `%NAME%` for cmd, `${NAME}` for POSIX shells). The secret value stays in the environment and never enters `argv`.
- Translate only when the target shell is known and the tag names an effective key; a tag with no matching key passes through untouched.
- Refuse loudly instead of silently misbehaving: if a matching tag is inside a single-quoted region (where the shell will not expand the reference) or the shell is unknown/empty, `execute_with_secrets` returns a clear error telling the agent to use the native reference or pass `shell`.
- Extend `get_context` with an optional `shell` argument and a `secret_reference` field (`shell`, `native`, `tag`) for the resolved shell, defaulting to the OS shell, as an informational fallback.
- Update the server instructions to describe the flow: discover keys, read the context/reference, then execute or proxy.
- Non-goals: `blindenv run` (no shell parameter; the CLI user already writes native syntax), changing `proxy_http_request`, changing environment-variable naming, or substituting values into commands.

## Capabilities

### New Capabilities
<!-- None: this modifies the existing MCP server behavior. -->

### Modified Capabilities
- `mcp-server`: `execute_with_secrets` translates the `{{SECRET_NAME}}` tag to the shell-native env reference (value never in `argv`) and fails loudly when it cannot; `get_context` reports the resolved shell's secret reference; the discovery guidance covers the reference step.

## Impact

- `pkg/mcp/exec.go` — shell-aware, quote-aware tag translation before invoking the shell; loud errors.
- `pkg/mcp/tools.go` — `get_context` `shell` argument and `secret_reference` output.
- `pkg/mcp/server.go` — instructions and shell/reference helpers.
- `pkg/mcp/shell.go` (new, optional) — shell classification and reference mapping.
- Tests in `pkg/mcp/server_test.go` / `pkg/mcp/exec_test.go`; README notes.
