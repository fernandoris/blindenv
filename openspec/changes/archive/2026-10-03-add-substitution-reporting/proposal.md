## Why

The secret-consuming tools report only `redactions`, a response-side count, so `redactions: 0` is ambiguous: it is produced both when a secret was correctly substituted into the request and the server simply did not echo it (for example a Rancher 401), and when no `{{SECRET_NAME}}` tag was ever replaced (a typo, an unused secret, or a literal tag sent to the server). An operator cannot tell an auth failure apart from a BlindEnv injection failure.

## What Changes

- Add a request-side `substitutions` count (number of `{{SECRET_NAME}}` occurrences replaced) to the results of `proxy_http_request`, `execute_with_secrets` and `open_in_browser`, so it reads alongside the existing response-side `redactions`.
- Add `unmatched_tags` (the names, never the values, of `{{...}}` tags that do not name an effective key) to the same three results, capped in length, to flag a mistyped or unknown key directly.
- Persist the substitution count in the audit log via an additive migration (new `substitutions` column, `schemaVersion` bump); `unmatched_tags` is not persisted.
- Clarify the terminology of `StreamRedactor.Count`, whose doc comment calls redactions "substitutions", to avoid a new collision with the request-side meaning.

## Capabilities

### New Capabilities
<!-- None. -->

### Modified Capabilities
- `mcp-server`: the result payloads of `proxy_http_request`, `execute_with_secrets` and `open_in_browser` gain `substitutions` and `unmatched_tags`; the audit requirement gains the persisted substitution count.

## Impact

- `pkg/mcp/proxy.go`, `pkg/mcp/exec.go`, `pkg/mcp/open.go` (result structs and counting), `pkg/mcp/redact.go`/`shell.go`-adjacent substitution helpers (`substitute`, `substituteBody`, `translateSecretTags`).
- `pkg/mcp/server.go` and `pkg/db/models.go`, `pkg/db/schema.go`, `pkg/db/store.go` (additive audit column `substitutions`), `pkg/web/server.go` (audit view).
- `pkg/mcp/perf_test.go` (new counting benchmark) and tool tests.
