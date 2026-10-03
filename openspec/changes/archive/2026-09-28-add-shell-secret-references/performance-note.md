# Performance note: shell tag translation

Scope: the `{{SECRET_NAME}}` translation added to `execute_with_secrets`
(`pkg/mcp/shell.go`), which runs on every command that contains a `{{` tag.

## Method

```
go test ./pkg/mcp/ -run '^$' -bench 'Translate' -benchmem
go test ./pkg/mcp/ -run TestTranslateNoTagsAllocs
```

Environment: darwin/arm64, Apple M5 Pro.

## Results

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkTranslateOneTag` (realistic curl, 1 tag) | 286.6 | 112 | 1 |
| `BenchmarkTranslateTagCount/tags=1` | 54.44 | 24 | 1 |
| `BenchmarkTranslateTagCount/tags=10` | 435.1 | 176 | 1 |
| `BenchmarkTranslateTagCount/tags=100` | 4046 | 1792 | 1 |
| `BenchmarkTranslateManyKeysNoTags` (100 keys, no tags) | 12.47 | 0 | 0 |

`TestTranslateNoTagsAllocs` reports 0 allocations for a tagless command even
with 100 effective keys, guarding the fast path.

## Interpretation

- **Linear in tag count.** tags=1 -> 10 -> 100 scales ~54 ns, ~435 ns, ~4046 ns
  (roughly 40 ns per additional tag), with a single output-buffer allocation
  regardless of tag count.
- **No per-key cost.** The no-tag path returns immediately (`strings.Contains`
  guard) and allocates nothing, so it cannot regress to the O(keys x length)
  shape that a `for key: strings.ReplaceAll` loop would have (one full-command
  copy per key).
- **Negligible against execution.** Translation is sub-microsecond even for 100
  tags; `execute_with_secrets` spends milliseconds spawning a process. The added
  overhead is not observable.

## Conclusion

The translation is O(command length) in a single pass with one allocation when
it rewrites and zero when it does not, so it adds no meaningful cost to the
execute path.
