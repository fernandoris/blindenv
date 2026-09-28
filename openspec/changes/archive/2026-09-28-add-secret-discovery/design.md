## Context

See `proposal.md` for motivation. The relevant current behavior:

- `effectiveSecrets` (`pkg/db/store.go`) selects rows with `(project_id = ? OR project_id IS NULL) AND (environment = ? OR environment IS NULL)`. With `environment = ""` it matches only NULL-environment rows, so every environment- and project_environment-scoped key disappears from `list_secret_keys`, `get_context`, `execute_with_secrets` and `proxy_http_request`.
- `resolveScope` validates a non-empty environment against the project's `environments` table, so a shared environment name that no project defines errors instead of listing.
- The scope-complete view already exists for the UI: `handleState` (`pkg/web/server.go`) assembles `ScopeSecrets(project, "")`, `ScopeSecrets(project, env)` per environment, `ScopeSecrets("", "")` and `ScopeSecrets("", name)` per shared environment (`pkg/db/store.go`), plus `ListSharedEnvironments`.
- `resolveContext` (`pkg/mcp/server.go`) allows an empty environment without warning.

## Goals / Non-Goals

**Goals:**
- Make every existing key discoverable for a project regardless of the resolved environment, without ever returning values.
- Reuse the existing scope enumeration rather than add storage or a new query shape.
- Make the resolved environment explicit for the tools that inject values.

**Non-Goals:**
- Changing `list_secret_keys` or `get_context` response shapes.
- Returning values from discovery, or any reveal capability.
- Changing the four-tier precedence or the storage schema.
- Cross-project discovery in the default tool (shared scopes are included; other projects are not).

## Decisions

### 1. New tool `discover_secrets` instead of extending `list_secret_keys`

A separate read-only tool keeps `list_secret_keys` a minimal, cheap, names-only call (the reason provenance was moved to `get_context` in `add-get-context-manifest`). Discovery is an explicit, one-time phase; annotating every listing call would add tokens to the most frequent call.

Alternative: add a `detail` flag to `list_secret_keys`. Rejected: same schema concern and it blurs two different intents.

### 2. Context-agnostic, all-scope view

Discovery enumerates keys defined across every applicable scope, independent of the resolved environment. This directly removes the silent omission that caused an injected key to be missing from the listing.

Alternative: keep the effective-per-context view and only annotate it. Rejected: annotation alone does not include keys filtered out before annotation.

### 3. Grouped response shape, names only

```
{
  "project": "rancher",
  "environment": "DES",                       // resolved context; may be ""
  "scopes": {
    "global": ["GLOBAL_TOKEN"],
    "environment": { "DES": ["RANCHER_SCOPE"], "prod": ["API_KEY"] },
    "project": ["REGION"],
    "project_environment": { "DES": ["DB_HOST"] }
  }
}
```

Scope groups live under `scopes` so the resolved project slug (`project`) and the project-global group do not collide. Grouping keeps the payload compact and matches the scope names used by `get_context` (`global`, `environment`, `project`, `project_environment`). Flat `[{key, scope, environment}]` is the alternative; rejected as more repetitive per key.

### 4. Scope: resolved project plus shared scopes

Discovery covers the resolved project (project-global and all its project + environment definitions) plus the two shared scopes (global, and environment-global for every name carrying such a key). It does not enumerate other projects; the agent is pinned to a project and shared scopes are what cross projects.

### 5. Reuse `ScopeSecrets` / `ListSharedEnvironments`

Add one store method that returns grouped key names for a project, built from the existing scope queries. No schema change, no new SQL semantics. The UI's `handleState` already assembles the same data, so the logic is proven.

### 6. Require an explicit resolved environment for secret-consuming tools (BREAKING)

`proxy_http_request` and `execute_with_secrets` refuse a call whose resolved environment is empty, instead of silently operating on global + project-global only. `discover_secrets`, `list_secret_keys` and `get_context` keep working without an environment. This closes the trap where execution and listing disagreed.

Alternative: keep the empty-environment behavior and only warn. Rejected: a warning does not prevent running with the wrong effective set.

## Risks / Trade-offs

- [Token growth with many keys or scopes] -> Names only, grouped, one call; a size cap or pagination can be added later without changing the spec.
- [Agent chooses the wrong environment after discovery] -> The response echoes the resolved environment, and secret-consuming tools now require one, so a mismatch is explicit rather than silent.
- [Breaking change for clients that omitted the environment] -> Clear error message; document in README and server instructions; the pinned `BLINDENV_ENV` keeps existing correct setups working.
- [Discovery overlaps `get_context`] -> `get_context` stays the effective, per-context view with provenance; discovery is the all-context inventory. Distinguish clearly in the tool descriptions.
- [Key names expose topology] -> Names were already exposed through `list_secret_keys`; values remain protected.

## Migration Plan

Additive tool plus an instruction update. The only behavioral change is the environment requirement on the two secret-consuming tools; clients that always set an environment or pin `BLINDENV_ENV` are unaffected. Rollback is reverting the handlers and instructions.

## Open Questions

- Whether a per-response size cap for discovery is needed before or after first release; deferrable without changing the specs.
