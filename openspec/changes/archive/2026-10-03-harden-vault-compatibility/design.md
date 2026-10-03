## Context

See proposal.md - Why. Two facts about the current code shape the approach:

- `migrate()` in `pkg/db/schema.go` decides what to do by structural introspection (`tableExists`/`columnExists`), never by reading `schema_version`. It is idempotent and runs on every `Open`.
- `recordSchemaVersion` unconditionally runs `UPDATE schema_version SET version = ?`, so the marker can regress. `schema_version` is otherwise never read.
- `pkg/crypto` is byte-for-byte unchanged since the initial commit, so all existing encrypted values remain decryptable. No migration touches the encryption format.
- The MCP server speaks JSON-RPC over stdio; stdout is reserved for the protocol.

## Goals / Non-Goals

**Goals:**
- An older binary must never modify a newer vault, and must say so clearly.
- Upgrading a vault must be automatic, non-interactive, and recoverable without the user having prepared a backup.
- The persisted schema version must reflect the true, highest format applied.
- Users must be told, once, on stderr, that a migration happened and where the rollback copy is.

**Non-Goals:**
- No in-place downgrade of a vault. Migration stays forward-only.
- No change to the encrypted vault format, key derivation, or the backup file format.
- No interactive confirmation prompts (the binary runs headless under agents).
- No automatic snapshot pruning/retention policy in this change.
- No rewriting of the existing v1→v2/v3/v4/v5 migrations.

## Decisions

### D1. Read the persisted version and refuse forward, before any write

`migrate()` first checks whether `schema_version` exists. If it does, it reads the stored version. If `stored > schemaVersion`, it returns a new sentinel `ErrSchemaTooNew` carrying both numbers, before `baseSchemaSQL` or any DDL runs, guaranteeing the vault bytes are untouched.

- Why: this is the only place that can protect against a stale binary silently rewriting a newer vault. The guard must precede all writes, including the version update.
- Alternative considered: rely on `schema_version` as a migration driver. Rejected - structural introspection is more robust (handles partially-applied and hand-edited vaults) and rewriting it is riskier than adding a guard.

### D2. Monotonic version marker

`recordSchemaVersion` advances the marker only: it never writes a value lower than what is stored (equivalently, it is only reached after passing the D1 guard, and it sets the version to `max(stored, schemaVersion)`).

- Why: defense in depth. Even if a future code path bypasses the guard, the marker cannot regress.
- Alternative considered: leave the unconditional update. Rejected - it is the mechanism of the current silent corruption.

### D3. Automatic pre-migration snapshot via `VACUUM INTO`, no passphrase

Before applying any migration that would change the schema (`stored` exists and `stored < schemaVersion`), the store writes a consistent copy `<vaultPath>.v<stored>.bak` using SQLite's `VACUUM INTO`.

- Why: the README currently pushes the whole risk onto the user ("irreversible, export a backup first"). A built-in, passphrase-free snapshot is the single biggest transparency win; the copy contains the same encrypted blobs as `vault.db`, so there is no new secret exposure.
- Why not a passphrase `Export`: `VACUUM INTO` needs no passphrase, works headless, and is a faithful encrypted copy, so it works even when only the OS keyring holds the key.
- Why name by source version: the snapshot represents the vault as it was before this upgrade; versioning avoids a later migration overwriting the rollback point.
- Only on real upgrades: a brand-new vault (no `schema_version` table) skips the snapshot. A vault already at `schemaVersion` skips it too.
- Collision: if `<vaultPath>.v<stored>.bak` already exists, it is left as-is (an existing rollback point is not clobbered).
- Permissions: created `0600` beside the vault in its `0700` directory.

### D4. One-time notice to stderr; keep `pkg/db` silent

`Open` stays free of I/O: it populates a small `MigrationInfo{From, To int, Snapshot string}` readable via `Store.Migration()`. `openVault` in `cmd/blindenv/main.go` prints one line to **stderr** when `From != To`:

```
blindenv: vault migrated v3 -> v5; backup at /.../vault.db.v3.bak
```

- Why: stdout must stay clean for MCP JSON-RPC. Keeping the db package I/O-free preserves layering and testability.
- Why one-time: after the migration the stored version equals `schemaVersion`, so subsequent opens print nothing and are silent.
- Alternative considered: pass a logger into `Open`. Rejected - it changes the signature across `mcp`/`ui`/`run`/tests for no benefit; the command layer already centralizes vault opening.

### D5. Never prompt

Migration always proceeds when it is safe to do so. There is no "migrate now?" question.

- Why: `blindenv mcp` runs without a TTY. D1's guard plus D3's snapshot make proceeding safe.

### D6. Forward-only, with a documented recovery path

Migration remains forward-only. The recovery procedure: stop all BlindEnv processes, restore `<vault>.v<old>.bak` over `vault.db` (removing stale `-wal`/`-shm`), then run the desired binary. When the installed binary is newer than the vault, no action is needed.

- Why: an in-place downgrade would require reverse migrations for every format, which is disproportionate. A snapshot makes "restore and use an older binary" the supported, obvious path.

### D7. Resource bounds: no vault-in-memory snapshot, no steady-state overhead

The snapshot MUST be produced by SQLite itself (`VACUUM INTO`), so the Go process never reads the vault into memory; this keeps peak memory proportional to a single page buffer, not the vault size. When the vault is already at `schemaVersion` (the steady state), no snapshot is taken and `migrate()` only runs cheap idempotence probes plus a single version read.

- Why: the tool already commits to bounded memory elsewhere (see the `run` bounded-output requirement); buffering the whole vault before an upgrade would regress that property exactly when the vault is largest.

## Performance and Memory Verification

The repo already uses Go benchmarks (`Benchmark*` in `pkg/db/perf_test.go`, `pkg/backup/perf_test.go`) and allocation budgets (`testing.AllocsPerRun` in `pkg/mcp/capture_test.go`, `pkg/mcp/shell_test.go`). This change follows the same conventions:

- **Snapshot memory**: assert via `testing.AllocsPerRun` that taking the pre-migration snapshot does not allocate proportional to vault size (a large-vault seed with a low allocation ceiling), proving the copy is done inside SQLite.
- **Snapshot time**: a `BenchmarkMigrateSnapshot` over a seeded vault so the one-time upgrade cost is visible and comparable as the vault grows.
- **Steady-state overhead**: a `BenchmarkOpenCurrent` that opens an already-current vault, to show the guard/snapshot logic adds no meaningful cost on the hot path (every process start).
- **Regression guard**: a large-vault test asserting snapshot+open succeeds within a bounded wall-clock/alloc budget, so a future change cannot silently buffer the vault.

Thresholds are set generously (order-of-magnitude, not micro-benchmarks) to avoid flakiness across CI machines, matching the existing bounded-allocation tests.

## Risks / Trade-offs

- [Snapshot doubles vault disk usage at migration time] -> Created only on a version bump, not on every open; noted as a future retention policy in Open Questions.
- [`VACUUM INTO` requires SQLite >= 3.27] -> `modernc.org/sqlite v1.59.0` bundles a newer SQLite; verified available.
- [A stale binary now fails to start instead of limping] -> Intended: failing loudly with an actionable message is strictly better than silent corruption. The message states the vault was not modified.
- [MCP client may only show "connection closed" when the guard fires] -> The guard error is written to stderr before the server starts; see Open Questions for a possible JSON-RPC-level error.
- [Snapshot could be stale if a second process holds the vault] -> `VACUUM INTO` reads a consistent snapshot through the WAL; concurrent access is already handled by `busy_timeout` and WAL.

## Migration Plan

1. Land the guard, monotonic marker, snapshot, and notice.
2. On the first open after upgrade, a `stored < schemaVersion` vault is snapshotted to `<vault>.v<stored>.bak` and migrated in place; the user sees one stderr line.
3. Rollback of the deployment: stop processes, restore the snapshot over `vault.db` (delete `-wal`/`-shm`), reinstall the previous binary. Because the guard refuses newer vaults, no partial state is possible.
4. Update README to describe the automatic snapshot and the recovery procedure, replacing the manual pre-upgrade backup advice.

## Open Questions

- Snapshot retention: should only the last N snapshots be kept, and should a snapshot be pruned once the migration is confirmed successful? Deferrable; the copies are encrypted and bounded by vault size.
- Should the forward guard also surface as a structured MCP error (rather than only a stderr line and process exit) so an agent sees the reason without relying on client stderr capture? Deferrable; does not change the vault-side behavior or specs.
