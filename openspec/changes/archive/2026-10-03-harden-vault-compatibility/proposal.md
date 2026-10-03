## Why

Vault migrations run automatically on open, but the persisted `schema_version` is written and never read, so an older binary can silently open a newer vault and corrupt it (marker and semantics), and upgrades rely on the user remembering to export a backup first. We want upgrades to be transparent: automatic, self-protected, and impossible to corrupt silently.

## What Changes

- Read the persisted schema version on open and **refuse to open a vault whose schema is newer than the binary supports**, with an actionable error, without modifying the vault.
- Make the schema version marker **monotonic**: it only advances, never regresses.
- **Automatically snapshot the vault before applying any migration** using a consistent copy (`VACUUM INTO`) that needs no passphrase, so a failed or unwanted upgrade is recoverable with a file restore. This replaces the current "export a backup before upgrading" advice with a built-in safety net.
- Emit a **one-time migration notice to stderr** (never stdout, which carries MCP JSON-RPC) reporting the old→new version and the snapshot location.
- Never prompt interactively: the common case migrates with no user action.
- Document the downgrade/recovery procedure.

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `vault`: adds a schema-version compatibility requirement (forward guard, monotonic marker, automatic pre-migration snapshot) and documents forward-only migration and recovery.
- `cli`: adds a requirement that migration/upgrade notices are written to stderr and never to stdout, preserving the MCP JSON-RPC channel.

## Impact

- `pkg/db/schema.go` — migration entry point: read-version guard, monotonic marker, pre-migration snapshot.
- `pkg/db/store.go` — `Open` exposing migration info; new `ErrSchemaTooNew` sentinel.
- `cmd/blindenv/main.go` — print the one-time notice to stderr from `openVault`.
- `README.md` — replace manual-backup-before-upgrade advice with the automatic behavior and the recovery procedure.
- Tests: existing seeds for schema v1–v4 in `pkg/db/db_test.go` and `pkg/db/metadata_test.go` extend to cover the guard, monotonic marker, and snapshot.
- Performance and memory tests: benchmarks for the snapshot and steady-state open, plus allocation budgets proving the snapshot is not buffered in memory, following the existing `pkg/db/perf_test.go` and `testing.AllocsPerRun` conventions.
- No changes to `pkg/crypto`, the vault format, or the backup format; existing encrypted values continue to decrypt.
