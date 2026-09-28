## Why

`list_secret_keys` is scoped to a single resolved `(project, environment)`, and `effectiveSecrets` only matches environment-scoped rows when the queried environment is non-empty (`environment = ? OR environment IS NULL`). When the environment is unset or different, every `environment` and `project_environment` secret is silently omitted. An agent can therefore see a value injected and redacted (e.g. a shared secret for environment `DES`) while `list_secret_keys` never reveals that the key exists. Agents need a discovery phase that shows what exists independently of the resolved context, without ever returning values.

## What Changes

- Add a read-only MCP tool `discover_secrets` that enumerates the project's key names with their scope and environment, grouped by scope, and never returns values. It is agnostic to the resolved environment, so environment-scoped keys are always visible.
- Reuse the existing scope enumeration (`ScopeSecrets`/`ListSharedEnvironments`, already assembled by the UI in `handleState`) instead of adding storage or queries.
- Add discovery-phase guidance to the server instructions: discover first, then choose a context and use secrets through `proxy_http_request` / `execute_with_secrets`.
- **BREAKING** (MCP call contract): `execute_with_secrets` and `proxy_http_request` require an explicit resolved environment; a call that resolves to an empty environment is refused instead of silently running with only global and project-global secrets.
- `list_secret_keys` stays names-only and unchanged.

## Capabilities

### New Capabilities
<!-- None: discovery extends the existing MCP server capability. -->

### Modified Capabilities
- `mcp-server`: add the `discover_secrets` tool (names, scope and environment only; no values) and a discovery phase in the server guidance; require an explicit resolved environment for secret-consuming tools.

## Impact

- `pkg/mcp/tools.go` — register and implement `discover_secrets`.
- `pkg/mcp/server.go` — server instructions and resolved-environment validation.
- `pkg/mcp/exec.go`, `pkg/mcp/proxy.go` — refuse an empty resolved environment.
- `pkg/db/store.go` — expose a grouped, scope-complete view (may reuse `ScopeSecrets` / `ListSharedEnvironments`).
- Tests in `pkg/mcp/server_test.go` and `pkg/db/db_test.go`; README tool list and usage.
