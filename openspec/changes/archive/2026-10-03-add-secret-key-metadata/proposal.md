## Why

A stored value carries no information about *what it is* or *how to use it*, so both humans and agents must infer it by trial and error. In one incident an agent appended `/v3` to `RANCHER_API_ENDPOINT`, which already contained `/v3`, producing a silent `/v3/v3` failure. At the same time, every value is forced through encryption and redaction, so non-secret configuration such as a base URL or host cannot be read or used by an agent and can only be retrieved by guessing. BlindEnv conflates "secret" with "everything I put in the vault".

## What Changes

- Add optional per-definition metadata to each secret: `type` (closed enum: `url`, `host`, `connection-string`, `token`, `password`, `text`), `hint` (free-form usage guidance, never a value), and `sensitive` (boolean, default `true`).
- `sensitive=false` marks a definition as configuration: its value is stored **unencrypted** at rest, **not redacted** from command/HTTP output, and **readable by agents** through `get_context`.
- `sensitive=true` keeps today's behavior: encrypted at rest, redacted, never returned to agents.
- Expose `type`, `hint` and `sensitive` on every key in `get_context` and `discover_secrets` (names/metadata only; values only for `sensitive=false`).
- Web UI: capture `type`/`sensitivity`/`hint` on add, edit metadata on an existing definition without re-entering the value, show configuration values in plain text, render a type label and the hint per row, and let the filter match hint/type.
- Database migration to schema v3; backup format extended, backward compatible with existing backups (older backups import as `sensitive=true`, no type/hint).
- Add benchmarks covering resolution, redaction, scoped-key listing and backup round-trips with metadata.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `vault`: add per-definition metadata; introduce non-sensitive configuration values stored unencrypted; amend "Per-value encryption" to apply to sensitive values only; extend backup to carry metadata.
- `mcp-server`: `get_context` and `discover_secrets` return type/hint/sensitivity; `get_context` returns values for non-sensitive keys; agent-facing rendering reflects configuration values.
- `secret-redaction`: redaction applies only to sensitive values.
- `web-ui`: add/edit metadata, render configuration values in plain text, type label and hint per row, filter by hint/type.

## Impact

- Storage/schema: `pkg/db/schema.go`, `pkg/db/store.go`, `pkg/db/models.go` (columns `sensitive`, `kind`, `hint`, `value_plain`; schema v3; resolution carries metadata through).
- MCP: `pkg/mcp/tools.go`, `pkg/mcp/proxy.go`, `pkg/mcp/exec.go`, `pkg/mcp/redact.go`, `pkg/mcp/server.go` (redactor built from sensitive values only).
- Web UI/API: `pkg/web/server.go`, `web/app.js` (new metadata endpoint, view fields, row rendering).
- Backup: `pkg/backup/backup.go` (metadata in export/import; `magicV1`/`magicV2` still importable).
- Docs: `README.md` threat model and feature list must state that non-sensitive configuration values are intentionally not encrypted and are visible to agents.
