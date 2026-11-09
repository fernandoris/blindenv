# Design

## Context

See `proposal.md` - Why. The relevant current state:

- `secrets` already has `value_enc`, `value_plain`, `sensitive`, `kind`, `hint`, `created_at` and `updated_at` (`pkg/db/schema.go`). Updating a value needs no new storage.
- `writeSecret` (`pkg/db/store.go:706`) is an upsert: it updates the row selected by scope + key and inserts only when no row matches.
- `PutSecretMeta` resets metadata to whatever the caller sends; `SetSecretMetadata` (`pkg/db/store.go:650`) updates metadata only and knows how to re-encode a value when sensitivity flips.
- The web API already exposes metadata-only `PATCH` for both project and shared scopes (`pkg/web/server.go`), and the dashboard's edit dialog only edits metadata (`web/app.js:799`).
- There are no JavaScript tests; the UI is validated through the HTTP API tests. `make check` is the CI gate.

## Goals / Non-Goals

**Goals:**

- Edit value and metadata in one operation on an existing definition, preserving scope, key and creation time.
- Keep the HTTP change additive and the storage schema untouched, so older binaries keep working.
- Never reveal or return the current value as part of an edit.
- Prove with tests that an update remains O(1) in time and memory regardless of vault size.

**Non-Goals:**

- Renaming a key or moving a definition to another scope, project or environment.
- Auditing edits (create and delete are not audited today).
- Any MCP or CLI writing surface.

## Decisions

### D1: Extend the existing PATCH with an optional `value`

`PATCH /api/projects/<slug>/secrets` and `PATCH /api/shared/secrets` gain an optional `value` field; the request body uses a pointer (`*string`) so "absent" is distinguishable from "sent". Omitting it keeps the current metadata-only behavior.

- **Why**: additive and backward compatible. Existing clients keep working, and there is one scope-resolution path instead of two.
- **Alternative considered**: a dedicated `PUT .../secrets/value`. Rejected: duplicates the project/shared scope handling and cannot apply value and metadata changes in a single validated write.
- **Alternative considered**: reuse `POST` (upsert). Rejected: it silently creates a missing key and resets metadata that the caller omits.

### D2: A store operation that reads, merges and writes in a transaction

Add `UpdateSecret(ctx, project, environment, key, value *string, meta *SecretMeta) (short bool, error)`:

1. Resolve the scope and select the existing row in that scope only.
2. Return `ErrSecretNotFound` if it does not exist (no implicit create).
3. Merge supplied fields over the existing value and metadata.
4. Normalize metadata, reject a hint containing the **effective** value, and reject an explicitly empty replacement value.
5. Call the existing `writeSecret` with the effective value and metadata, so encryption, sensitivity flips and the `created_at`/`updated_at` handling stay in one place.
6. Run the select and the write in one transaction to avoid a lost update between processes.

- **Why**: one validated write covers value-only, metadata-only and combined edits, and reuses the proven encryption path.
- **Alternative considered**: keep `SetSecretMetadata` and add a separate `SetSecretValue`. Rejected: two round trips and two validation paths; a combined edit would race with itself.
- **Trade-off**: a read-modify-write is slower than a blind `UPDATE`, but it is still a single-row operation and is bounded independently of the number of definitions. Concurrent edits follow last-writer-wins, consistent with the existing metadata edit path.

### D3: No schema change and no version bump

The edit uses existing columns and `updated_at`. `schemaVersion` stays as is.

- **Why**: software libre users may run an older binary against a newer vault. Without a migration there is nothing to snapshot, nothing to reverse, and no forward-only hazard.
- **Alternative considered**: add a `revision` or `edited_by` column. Rejected: changes the schema and downgrade story for no required behavior.

### D4: The value is write-only on the edit path

The edit request accepts a replacement value; the response returns only success and the short-value flag. The current value is never read out to the client for editing.

- **Why**: matches the existing explicit-reveal contract. Editing must not become a way to dump secrets.
- **Consequence**: the UI replacement field starts blank; blank means "keep", and the current value is never prefilled. This is consistent for sensitive and non-sensitive definitions and avoids accidental copying.

### D5: One dashboard dialog for value and metadata

Replace the metadata-only dialog with an "Edit secret" dialog containing an optional replacement-value input (accessible label plus a description that blank keeps the current value), the existing type/sensitivity/hint fields, and the short-value warning after save.

- **Why**: the user asked to edit "a secret", not a value and metadata through two different flows. One save also avoids a metadata write followed by a value write.
- **Alternative considered**: an inline value field per row. Rejected: pairs poorly with the existing metadata editor and increases the chance of accidental submission.

### D6: Hint is revalidated against the effective value

Whether the hint is supplied or preserved, it is checked against the merged value before the write.

- **Why**: changing the value can make a previously safe hint contain the new secret; the metadata channel must not become a leak.

### D7: Tests include functional, API, benchmark and bounded-allocation coverage

- Store tests: identity preservation, re-encryption, non-sensitive, sensitivity flip, missing key, hint rejection, short warning.
- API tests: PATCH with and without `value`, project and shared scopes, response never contains the value.
- Benchmark `BenchmarkSecretValueUpdate` and an allocation test comparing a small and a large vault, following the existing `perf_test.go` pattern.
- Compatibility: reuse the historical vault seeds and edit after opening.

## Risks / Trade-offs

- [Concurrent edits across processes can lose an update] -> Wrap the read-modify-write in a transaction; document last-writer-wins as the existing behavior for metadata edits.
- [A value edit could reset metadata] -> Merge over the stored metadata instead of constructing defaults; keep a regression test for metadata-only patches and for combined edits.
- [Mixed versions: an older binary ignores the new `value` field] -> The dashboard and API are served by the same binary, so this only affects external API clients; document that editing values requires a current build. The vault itself remains readable by older builds.
- [A preserved hint may now contain the new value] -> Revalidate the merged hint before writing and reject with `ErrInvalidMetadata`.
- [Update cost grows with vault size] -> The operation touches one row; add a benchmark and a bounded-allocation test to keep it that way.

## Migration Plan

1. Ship the additive API/store change and the dashboard dialog. No schema migration, no snapshot, no backup change.
2. Deploy the new binary; existing vaults need no action.
3. Rollback: restore the previous binary. Edited definitions remain valid because they use the same columns and representation the previous binary already understands.

## Open Questions

None.
