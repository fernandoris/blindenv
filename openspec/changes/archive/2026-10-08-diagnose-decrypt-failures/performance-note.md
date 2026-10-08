# Performance and memory note — diagnose-decrypt-failures

Scope: the master-key verification added at unlock (`Store.VerifyMasterKey`),
which runs once per process start when the vault has sensitive definitions. It
streams `secrets` rows and decrypts until the first success, so a matching key
costs a single AES-GCM decryption and the failure path is the only one that
visits every row.

Environment: Apple M5 Pro, darwin/arm64, go1.27.1. Command:

```
go test ./pkg/db/ -run '^$' -bench 'UnlockVerify|OpenCurrent' -benchmem -benchtime=200x -count=3
```

## Results (median of 3)

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkUnlockVerifyMatching` (200 sensitive rows) | ~4,300 | 1,984 | 24 |
| `BenchmarkUnlockVerifyMismatch` (200 rows, no decrypt succeeds) | ~134,000 | 394,104 | 2,015 |
| `BenchmarkOpenCurrent` (steady-state open, for reference) | ~465,000 | 18,200 | 681 |

Allocation budget (`TestUnlockVerifyAllocationsBounded`):

| Vault size | allocs/op |
| --- | ---: |
| 10 sensitive definitions | 24 |
| 500 sensitive definitions | 24 |

## Conclusion

- The matching-key path adds one decryption: ~4.3 µs and a constant 24
  allocations, about 1% of the ~465 µs steady-state open and independent of the
  number of definitions (24 allocs at both 10 and 500 rows). Memory does not
  scale with vault size, confirming the rows are streamed and the loop stops at
  the first success.
- The mismatch path is heavier (~134 µs, ~394 KB for 200 rows) because it
  visits every sensitive definition, but it only runs on a failure and is still
  far below any interactive threshold. It is the price of turning a misleading
  per-key integrity error into a correct master-key-mismatch diagnosis.
- No regression to the success path: classification during resolution reuses
  the decryptions `ResolveDetailed` already performs; only the abort-on-first
  behavior changes.
