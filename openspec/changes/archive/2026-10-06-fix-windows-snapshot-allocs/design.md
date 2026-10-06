## Context

See `proposal.md` - Why. `snapshotVault` (`pkg/db/schema.go:196`) issues a single
`VACUUM INTO ?` through the pure-Go `modernc.org/sqlite` driver, then
`os.Chmod`s the result. Because the driver allocates on the Go heap, the object
counts observed by `testing.AllocsPerRun` depend on its per-platform VFS/page
behaviour: macOS measured `64/64` (flat), Windows `85/409` (grows with the
seeded vault). The existing guard treats those counts as the invariant.

## Goals / Non-Goals

**Goals:**
- Express the real invariant - the vault is streamed by SQLite, never buffered
  as a Go slice - with a metric that is stable on Linux, macOS and Windows.
- Still fail if `snapshotVault` grows to read the vault into process memory.

**Non-Goals:**
- Change `snapshotVault` or any vault behaviour.
- Bound wall-clock migration time; `TestLargeVaultMigrationBounded` owns that.

## Decisions

**Decision 1: measure allocated bytes, not object count.**
Replace `testing.AllocsPerRun` with a helper that snapshots the vault a few
times and reads `runtime.MemStats.TotalAlloc` deltas (bytes per snapshot).
Rationale: an implementation that buffers the vault allocates on the order of
the vault size in bytes; bytes is the invariant, while object count is a driver
detail that already diverges by platform.
Alternatives considered: relaxing the object-count ratio per OS (still measures
the wrong thing and hard-codes magic numbers); `t.Skip` on Windows (drops
coverage exactly where behaviour differed).

**Decision 2: assert against vault size, with a loose factor as a backstop.**
The large snapshot must allocate well under the large vault's on-disk size
(e.g. `< size/2`) and not more than a small constant factor of the small
snapshot plus a margin. Keep a generous absolute object-count ceiling as a
secondary guard against row-by-row buffering.

## Risks / Trade-offs

- The pure-Go driver may allocate proportionally to vault size in bytes on some
  platform. Mitigate by asserting against the vault size with a generous
  divisor and validating on the Windows CI runner; if bytes also scale there,
  fall back to a Windows-scoped relaxation while keeping the absolute byte
  ceiling everywhere.
- `TotalAlloc` includes unrelated allocations. Mitigate by taking deltas across
  only the snapshot loop, with the store already open and otherwise idle.
