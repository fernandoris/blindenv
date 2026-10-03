# Performance note: add-secret-key-metadata

## Environment

- Apple M5 Pro, macOS (darwin/arm64), Go 1.27.1
- SQLite via `modernc.org/sqlite` (pure Go), WAL, `MaxOpenConns(4)`
- `benchstat` v0.0.0-20260929

## Method

`BenchmarkResolveAllSensitive` seeds 200 keys in all four scopes (800 rows) as
sensitive and resolves one project + environment. It uses only APIs that exist
both before and after this change, so it is a valid before/after comparison.

- Baseline: `git worktree` at the pre-change commit (`6f1e8c1`), equivalent
  benchmark, `-benchtime=300x -count=8`.
- After: this working tree, same benchmark, same flags.

```
go test ./pkg/db/ -run '^$' -bench '^BenchmarkResolveAllSensitive$' -benchmem -benchtime=300x -count=8
go run golang.org/x/perf/cmd/benchstat@latest baseline.txt after.txt
```

## Before / after (hot resolution path)

```
                       │ baseline.txt  │         after.txt         │
                       │    sec/op     │   sec/op     vs base      │
ResolveAllSensitive-15      439.2µ ± 3%   673.2µ ± 7%  +53.27% (p=0.000 n=8)

                       │    B/op       │    B/op      vs base      │
ResolveAllSensitive-15     687.7Ki ± 0%  930.6Ki ± 0%  +35.31% (p=0.000 n=8)

                       │   allocs/op   │  allocs/op   vs base      │
ResolveAllSensitive-15      10.49k ± 0%   15.50k ± 0%  +47.79% (p=0.000 n=8)
```

## Post-change benchmarks (metadata-aware paths)

```
BenchmarkEffectiveSecretsMixed-15     ~595µs/op   768.9 KiB/op   14.7k allocs/op
BenchmarkListScopedKeys-15            ~6.40ms/op  1,073 KiB/op    26.2k allocs/op
BenchmarkRedactorMixedSensitivity-15  ~100µs/op   15.8 KiB/op      203 allocs/op
BenchmarkExportImport/export-15       ~25.4ms/op  67.3 MiB/op      2.1k allocs/op
BenchmarkExportImport/import-15       ~34.2ms/op  67.5 MiB/op      8.7k allocs/op
```

Notes:

- `BenchmarkEffectiveSecretsMixed` resolves the same 800-row shape with half the
  definitions non-sensitive; resolution is *cheaper* than the all-sensitive case
  because non-sensitive definitions are not decrypted.
- `BenchmarkListScopedKeys` retains its shape: one `ScopeSecrets` query per
  shared environment and per project environment (the pre-existing N+1). The
  extra columns are constant per query; the asymptotic profile is unchanged.
- `BenchmarkExportImport` is dominated by Argon2id key derivation per call, not
  by JSON. The sparse `Metadata` map adds no measurable cost at this size.
- `BenchmarkRedactorMixedSensitivity` confirms the redactor is built from the
  sensitive-only map: 100 sensitive values, and a configuration value in the
  payload is left untouched.

## Analysis and accepted trade-off

The hot resolution query now selects four additional columns (`value_plain`,
`sensitive`, `kind`, `hint`). Isolating them showed the cost is roughly split
between the two text columns (`kind` + `hint`, ~116µs / ~3.2k allocs over 800
rows) and the value columns (`value_plain` + `sensitive`, ~56µs / ~0.8k allocs).

- The regression is **linear and bounded**: ~0.29µs and ~6 allocations per
  scanned definition. A vault with 100 definitions pays roughly 30µs, which is
  negligible next to the SQLite query and the AES-GCM decrypt of each value.
- The benchmark is a worst case: every key is defined in all four scopes, which
  quadruples the scanned rows for a fixed number of effective keys.
- This is the cost of exposing type, hint and sensitivity to agents in the same
  resolution pass, and of supporting cleartext configuration values.

**Accepted**: ship as is. The metadata is small, the overhead is linear and
sub-microsecond per definition, and the alternative (deferring `kind`/`hint` to
a second per-scope query that fetches only the winning definitions) trades this
simplicity for a saving that is invisible at realistic vault sizes.

**Deferred optimization** (if a large vault ever makes resolution hot): move
`kind`/`hint` loading into a second query restricted to the winning
(scope, key) pairs, leaving the candidate scan at `key, value_enc,
value_plain, sensitive, project_id, environment`. A prototype reduced the
all-sensitive case to ~557µs; it was not adopted because the added query and
grouping complexity is not justified by the absolute gain.
