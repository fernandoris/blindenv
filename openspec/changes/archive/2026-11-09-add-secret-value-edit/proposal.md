# Proposal

## Why

Changing a secret's value currently forces the user to delete the definition and create it again, which loses the definition's identity (`created_at`), invites accidental metadata resets, and makes a routine rotation feel destructive. The vault already stores an `updated_at` and can update a row in place, but neither the store API nor the dashboard exposes a safe "edit this secret" operation.

## What Changes

- Allow editing a secret **value and metadata together, in place, without deleting or re-creating the definition**. The scope, key, `created_at`, and any metadata not being changed are preserved; `updated_at` advances and a sensitive value is re-encrypted with a fresh nonce.
- Extend the existing metadata-only `PATCH /api/projects/<slug>/secrets` and `PATCH /api/shared/secrets` with an **optional** `value` field. Omitting `value` keeps today's metadata-only behavior; sending it replaces the value. The response never contains the value.
- Add a store operation that performs the update as a read-modify-write, merges the supplied fields over the existing definition, validates the hint against the **effective new** value, and refuses to edit a key that does not exist in that scope (no implicit creation).
- Unify the dashboard's "Edit metadata" dialog into an "Edit secret" dialog with an optional replacement-value field; a blank field keeps the current value and no reveal is required.
- Preserve all compatibility guarantees: **no schema change, no migration, no backup-format change, no MCP or CLI change**. Older binaries continue to read and operate a vault edited by a newer one, and agents still cannot write secrets.

Out of scope (deliberately):

- Renaming a key. It changes shadowing between scopes and breaks `{{TAG}}` references; it deserves its own change.
- Moving a definition between scopes, projects, or environments.
- Auditing edits. Creation and deletion are not audited today; edits stay at parity.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `vault`: define in-place update of a definition's value and metadata, preserving scope, key, and creation time; require that an update target an existing definition and that a hint never contain the effective value.
- `web-ui`: define editing an existing definition's value and metadata together without re-entering or revealing the current value, plus the existing metadata-edit behavior extended to the value.

## Impact

- Storage: `pkg/db/store.go` gains an in-place update operation built on the existing `writeSecret` upsert path; **no changes** to `pkg/db/schema.go`, the schema version, or backup.
- Web API/UI: `pkg/web/server.go` (optional `value` in the PATCH bodies), `web/app.js` (unified edit dialog).
- Docs: `README.md` dashboard feature description.
- Tests: functional store and API tests, plus benchmark and bounded-allocation coverage proving an update stays O(1) and does not scale with vault size.
