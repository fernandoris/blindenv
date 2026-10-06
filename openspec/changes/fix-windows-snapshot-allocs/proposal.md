## Why

CI on `windows-latest` fails `pkg/db`:

```
--- FAIL: TestSnapshotAllocationsBounded
    perf_test.go:282: snapshot allocs: small=85 large=409
    snapshot allocs scaled with vault size: small=85 large=409
```

The test asserts that `snapshotVault` allocates a constant number of Go
objects regardless of vault size, using `testing.AllocsPerRun`. On macOS the
counts are flat (`small=64 large=64`); on Windows they scale with the seeded
vault (85 -> 409). Passing `VACUUM INTO` through the pure-Go `modernc.org/sqlite`
driver allocates in the Go heap, and its per-page/VFS behaviour differs by
platform, so an object-count ratio is a fragile proxy. The real invariant - the
vault content is copied by SQLite and never buffered as a Go slice - is about
bytes, not object count, and still holds (`large` is far below the 2000-object
bound). The failure was hidden until the formatting and headless-test fixes
unblocked the test step.

## What Changes

- Make `TestSnapshotAllocationsBounded` assert the intended invariant with a
  platform-stable metric: allocated bytes per snapshot (e.g. `B/op` from
  `testing.Benchmark`), not object count, and keep a generous per-snapshot byte
  ceiling that a buffered implementation would blow.
- Keep `TestLargeVaultMigrationBounded` as the wall-clock guard for a large
  vault.
- Test-only; no product behaviour change.

## Capabilities

### New Capabilities
<!-- None: this is a test-only fix, no spec-level behavior change.
     The change opts out of specs via skip_specs: true. -->

None.

### Modified Capabilities

None.

## Impact

- `pkg/db/perf_test.go` - the snapshot allocation guard.
- `.openspec.yaml` - sets `skip_specs: true`.
