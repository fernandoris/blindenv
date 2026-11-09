# Performance and memory note

Environment: macOS (arm64), Apple M5 Pro, Go toolchain from `go.mod`.

## Benchmark: in-place value update

Command:

```sh
go test ./pkg/db -run '^$' -bench BenchmarkSecretValueUpdate -benchmem -count=3
```

Result (three runs):

```
BenchmarkSecretValueUpdate-15    21692    54350 ns/op    5230 B/op    106 allocs/op
BenchmarkSecretValueUpdate-15    21740    54557 ns/op    5230 B/op    106 allocs/op
BenchmarkSecretValueUpdate-15    21722    54278 ns/op    5229 B/op    106 allocs/op
```

The benchmark rotates a single key in a vault that also holds 50 unrelated
definitions.

## Bounded allocations

Command:

```sh
go test ./pkg/db -run TestSecretUpdateAllocationsBounded -v
```

Result:

```
secret update allocs: n=10 106, n=1000 104
```

Allocations per update are effectively constant: growing the vault from 10 to
1000 unrelated definitions does not increase the cost of an update. This is
expected because the update selects one row through the existing scope index
and rewrites only that row in a transaction.

## Security note

The edit path never returns the stored value. The request accepts an optional
replacement value; the response carries only the key and the short-value flag.
