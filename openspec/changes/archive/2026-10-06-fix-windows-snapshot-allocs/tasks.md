## 1. Replace the metric

- [x] 1.1 Add a `snapshotAllocBytes(t *testing.T, n int) uint64` helper to `pkg/db/perf_test.go` that opens a legacy vault of `n` rows and returns `runtime.MemStats.TotalAlloc` deltas divided by the number of snapshots; verify it compiles and returns a stable value on repeated local runs
- [x] 1.2 Rewrite `TestSnapshotAllocationsBounded` to compare bytes, not objects: assert large-snapshot bytes are below the large vault's on-disk size (e.g. `< size/2`) and within a small factor of the small-snapshot bytes plus a margin; keep a generous absolute object-count ceiling as a backstop. Verify `CGO_ENABLED=0 go test -run TestSnapshotAllocationsBounded -v ./pkg/db` passes and logs the byte figures

## 2. Cross-platform verification

- [x] 2.1 Run `make check` locally and confirm it succeeds
- [x] 2.2 Push and confirm `pkg/db` passes on `windows-latest`; if the byte counts still scale there, apply the design's Windows-scoped fallback (keep the absolute byte ceiling) and document why in the test comment

## 3. Documentation

- [x] 3.1 Update the test comment to explain why bytes are measured and note the driver/VFS platform caveat, replacing the object-count rationale
