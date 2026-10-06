## 1. Add a test seam for the display guard

- [x] 1.1 In `pkg/mcp/open.go`, replace the inline `hasDisplay(runtime.GOOS, os.Getenv)` call with an overridable package var, e.g. `var displayAvailable = func() bool { return hasDisplay(runtime.GOOS, os.Getenv) }`; verify `go build ./...` and `go test ./pkg/mcp` stay green (behavior unchanged)
- [x] 1.2 Add a test helper in `pkg/mcp/open_test.go` that forces `displayAvailable` to true and restores it on cleanup, mirroring `captureLauncher`

## 2. Fix the affected tests

- [x] 2.1 Update `TestOpenResolvesSensitiveTagWithoutLeak`, `TestOpenNonMatchingTagPassesThrough` and `TestOpenReportsSubstitutionsAndUnmatched` to use the helper so they no longer fail on a headless runner; verify `CGO_ENABLED=0 go test -run 'TestOpen' ./pkg/mcp` passes
- [x] 2.2 Confirm `TestOpenHeadlessLinuxRefused` still asserts refusal (its forced `displayAvailable=false` path) and `TestHasDisplay` is unchanged; verify both pass

## 3. Verify

- [x] 3.1 Run `make check` and confirm it succeeds on this machine
- [x] 3.2 Simulate headless Linux locally by asserting the guard path: `CGO_ENABLED=0 go test -run 'TestOpenHeadless|TestOpenResolves|TestOpenNonMatching|TestOpenReports' -v ./pkg/mcp` shows the three success tests passing and the headless test refusing
