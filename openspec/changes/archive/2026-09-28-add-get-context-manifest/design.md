## Context

See `proposal.md` for motivation and `specs/` for the required behavior.

Current state, grounded in the code:

- `handleGetContext` (`pkg/mcp/tools.go:77`) resolves project/environment, calls `Store.ListKeys`, and returns `os, arch, shell_hint, project, environment, secret_keys` — names only.
- `Store.ListKeys` (`pkg/db/store.go:530`) already resolves the project scope internally via `resolveScope` -> `projectID` -> `GetProject`, so the project row is fetched on every call; the `allow_execute` column is loaded but discarded.
- `Store.ListSecrets` (`pkg/db/store.go:549`) returns `[]SecretInfo{Key, Scope, Environment, Overrides}` and `SecretInfo` already carries the winning scope and the broader scopes it shadows (`pkg/db/models.go:41`). This is the provenance the web UI and audit already use.
- The four scope enum values are `global`, `environment`, `project`, `project_environment` (`pkg/db/models.go:10`). `allow_execute` is the shared name across the schema, the web API (`pkg/web/server.go:200`) and backups (`pkg/backup/backup.go:39`).
- Existing consumers of the response: `pkg/mcp/server_test.go:76` and `cmd/blindenv/integration_test.go:154`. Both decode into `map[string]any`.

## Goals / Non-Goals

**Goals:**

- `get_context` reports the execution capability for the resolved project before the agent calls `execute_with_secrets`.
- `get_context` reports each effective key's source scope, its source environment when environment-specific, and the scopes it shadows, so the agent can tell a shared key from a project-local one and judge where it resolves.
- Reuse existing provenance computation; no new storage, resolution, or execution behavior.

**Non-Goals:**

- Changing `proxy_http_request` or `execute_with_secrets`.
- Changing `list_secret_keys` (it stays names-only).
- Exposing secret values, or adding a general capability framework for a single flag.

## Decisions

### 1. `allow_execute` as a flat boolean, named after the existing field

Return `"allow_execute": <bool>` at the top level. It matches the schema column, the web `projectView` and the backup field, so MCP, UI and backups speak one name. The alternative — a `capabilities: { execute_with_secrets: bool }` envelope — is more extensible but speculative with only one gate, and it introduces a second vocabulary for the same concept. Rejected for now; revisit when a second gate appears.

### 2. Per-key provenance as objects, sourced from `ListSecrets`

Change `secret_keys` from `["API_KEY", ...]` to a list of objects:

```json
"secret_keys": [
  { "key": "API_KEY", "scope": "global" },
  { "key": "DB_HOST", "scope": "environment", "environment": "staging" },
  { "key": "REGION",  "scope": "project_environment", "overrides": ["global"] }
]
```

- `scope` is one of the four enum values, so the agent can derive reach: `global` everywhere, `environment` for that environment name only, `project` for one project, `project_environment` for one project + environment.
- `environment` is included only for the `environment` scope.
- `overrides` lists the broader scopes this definition shadows, and is included only when non-empty, to keep the payload lean.

`handleGetContext` calls `Store.ListSecrets` instead of `Store.ListKeys`; `SecretInfo` maps directly to these fields. Alternative considered: a parallel `key_scopes` map keeping `secret_keys` as strings — rejected because shadowing (`overrides`) is a list per key and does not fit a flat map cleanly, and the object form is self-describing for the model.

### 3. Provenance lives in `get_context`, not `list_secret_keys`

`get_context` is the context/manifest tool; `list_secret_keys` stays a minimal cheap call. Adding the same payload to both would duplicate tokens for every listing call. The top-level `environment` remains the active environment; the per-key `environment` is the source environment, and the spec/scenario wording distinguishes them.

### 4. Capability is advisory, not authorization

`allow_execute` in the response is a preflight signal. `execute_with_secrets` keeps its own check against the project row (`pkg/mcp/exec.go:56`) because the UI can toggle the flag between the two calls. The error message is unchanged.

## Risks / Trade-offs

- [Breaking response shape: `secret_keys` changes type] → the only consumers are the two tests in this repo; update both and document the shape in the README.
- [Vault topology disclosure] → scope names are already visible in the web UI and backups and are not values; the incremental risk is negligible.
- [Agent over-trusts a stale `allow_execute`] → execution re-validates server-side; the field is documented as advisory.
- [Token growth for large vaults] → emit `environment`/`overrides` only when non-empty and keep `scope` a short enum.
- [Raw enum strings may read poorly to the model] → acceptable for a first cut; a friendlier label is a deferrable open question.

## Migration Plan

No data migration. The change ships in the binary; rollback is reverting the commit. Update the two tests and the README's MCP defaults/feature bullets.

## Open Questions

- Whether to expose a friendlier source label (e.g. "environment-global") alongside the raw enum. Deferrable; the enum is already unambiguous and does not change the specs or task breakdown.
