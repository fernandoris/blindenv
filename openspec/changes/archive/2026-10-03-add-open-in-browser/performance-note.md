# Performance note: add-open-in-browser

## Environment

- Apple M5 Pro, macOS (darwin/arm64), Go 1.27.1

## What the open path costs

`open_in_browser` does three BlindEnv-owned things before handing the URL to the
OS launcher:

1. `GetProject` + `ResolveDetailed` — the same resolution path every
   secret-consuming tool uses; unchanged by this change.
2. `substitute(url, secrets)` — string replacement of `{{KEY}}` tags.
3. One `os/exec` spawn of the platform launcher, with the URL as the sole
   argument, and no shell.

There is no output buffering and no redaction pass: the tool returns only
`{"opened": true}`, so nothing proportional to the child's behavior enters
memory. Memory is O(URL length).

## Measurement

```
go test ./pkg/mcp/ -run '^$' -bench '^BenchmarkSubstituteURL$' -benchmem -benchtime=200000x

BenchmarkSubstituteURL-15    200000    224.9 ns/op    420 B/op    3 allocs/op
```

The substitution step is a few hundred nanoseconds over a representative URL
with three tags, with a constant number of allocations. The `os/exec` spawn is a
fixed platform cost (single-digit milliseconds) that a Go micro-benchmark does
not represent faithfully, so `BenchmarkSubstituteURL` deliberately covers only
the portion BlindEnv owns.

## Why the meaningful test is security, not a benchmark

A benchmark of "open a browser" measures the OS launcher, not BlindEnv, and
would be theater. The property that matters for this feature is that the
**resolved URL never reaches the agent, the audit log, any error, or a
shell**. That is asserted by `TestOpenResolvesSensitiveTagWithoutLeak`,
`TestOpenErrorDoesNotContainResolvedURL`, `TestResolveLauncherNeverUsesShell`
and `TestOpenDisabledByDefault` in `pkg/mcp/open_test.go`, not by a benchmark.

## Accepted residual leaks (not performance)

- The resolved URL is visible in the launcher's `argv` (and therefore `ps`) for
  the brief lifetime of the launcher process. Unavoidable: the OS launchers
  accept the URL only as an argument.
- The default browser persists the URL in its history; no private window is
  possible through the default handler.
- An agent with browser automation can read the value back from the tab.
