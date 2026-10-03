## 1. Compatibility guard

- [x] 1.1 Add an `ErrSchemaTooNew` sentinel and a `readSchemaVersion` helper that returns the persisted version (0 when the `schema_version` table is absent or empty); verify with a unit test in `pkg/db`.
- [x] 1.2 Add the forward guard at the start of `migrate()` (after `baseSchemaSQL`, before any other DDL): when the stored version is greater than `schemaVersion`, return `ErrSchemaTooNew` with both versions; verify a test seeds a `schemaVersion+1` vault, opens it, and asserts the error plus an unchanged stored version and table set.
- [x] 1.3 Make `recordSchemaVersion` advance only (never lower the stored version); verify with a test that pre-seeds a higher version and asserts the marker does not regress.

## 2. Pre-migration snapshot

- [x] 2.1 Implement the pre-migration snapshot with `VACUUM INTO` to `<vaultPath>.v<stored>.bak` (`0600`), taken only when the `schema_version` table pre-exists and `stored < schemaVersion`; verify tests cover: upgrade creates a snapshot, fresh vault creates none, current vault creates none, and an existing same-source snapshot is preserved unchanged.
- [x] 2.2 Ensure a snapshot failure aborts the open before any DDL runs; verify a test pointing the vault at a non-writable snapshot location fails without altering the vault.

## 3. Migration info and CLI notice

- [x] 3.1 Populate an exported `MigrationInfo{From, To int, Snapshot string}` on `Store` during `Open` and expose it via `Store.Migration()`; verify a test reads it after an upgrade and after a no-op open.
- [x] 3.2 Emit a one-time notice from `openVault` in `cmd/blindenv/main.go` to stderr only when `From != To`, naming both versions and the snapshot path; verify `cmd/blindenv` tests assert the notice appears on stderr and never on stdout.

## 4. Correctness tests

- [x] 4.1 Extend the existing schema v1–v4 seed tests in `pkg/db/db_test.go` and `pkg/db/metadata_test.go` to assert the guard, the monotonic marker, and the snapshot outcome for each source version.
- [x] 4.2 Add a round-trip test that migrates a seeded vault, restores the snapshot over `vault.db` (removing `-wal`/`-shm`), reopens it, and asserts the pre-migration values and version; verify it passes.
- [x] 4.3 Add a `cli` test that runs `blindenv mcp` over a migrating vault and asserts stdout stays valid JSON-RPC while the notice appears on stderr; verify it passes.

## 5. Performance tests

- [x] 5.1 Add `BenchmarkMigrateSnapshot` in `pkg/db/perf_test.go` over a seeded vault so the one-time snapshot cost is observable; verify it runs with `go test -bench MigrateSnapshot ./pkg/db`.
- [x] 5.2 Add `BenchmarkOpenCurrent` that opens an already-current vault to show steady-state overhead is negligible; verify it runs with `go test -bench OpenCurrent ./pkg/db`.
- [x] 5.3 Add a large-vault regression test asserting snapshot + open completes within a generous wall-clock budget so a future change cannot silently buffer the vault; verify it passes locally and in CI.

## 6. Memory tests

- [x] 6.1 Add a `testing.AllocsPerRun` test asserting snapshot allocations do not scale with vault size (large-vault seed with a low allocation ceiling), proving the copy is done inside SQLite; verify it passes.
- [x] 6.2 Add a `testing.AllocsPerRun` test bounding allocations for opening an already-current vault; verify it passes.

## 7. Documentation

- [x] 7.1 Update the README upgrade section to describe the automatic snapshot and one-time stderr notice, replacing the manual "export a backup before upgrading" instruction; verify the rendered section matches the shipped behavior.
- [x] 7.2 Document the recovery/downgrade procedure (stop processes, restore `<vault>.v<old>.bak` over `vault.db`, delete `-wal`/`-shm`, run the desired binary); verify it is reachable from the README upgrade section.
- [x] 7.3 Note the forward-compatibility guard behavior (older binary refuses a newer vault without modifying it) in the README; verify the text matches the guard's error message.

## 8. Verification

- [x] 8.1 Run `go test ./...`, `go vet ./...`, and the new benchmarks; verify all pass and allocation budgets hold.
- [x] 8.2 Run `openspec validate harden-vault-compatibility --strict`; verify it passes.
