## 1. Fix formatting drift

- [x] 1.1 Run `gofmt -w pkg/db/db_test.go pkg/web/server.go`; verify `gofmt -d pkg/db/db_test.go pkg/web/server.go` prints nothing and the diff is alignment-only
- [x] 1.2 Verify no behavior change: `CGO_ENABLED=0 go test ./pkg/db ./pkg/web` passes

## 2. Add local formatting guard

- [x] 2.1 Add a `Makefile` with a `fmt` target (runs `gofmt -w .`) and a `check` target mirroring CI (`test -z "$(gofmt -l .)"`, then `go vet ./...`, `go build ./...`, `CGO_ENABLED=0 go test ./...`); verify `make check` succeeds on the fixed tree
- [x] 2.2 Temporarily reintroduce a misalignment and verify `make check` fails at the formatting step (then revert); document the observed failure

## 3. Documentation

- [x] 3.1 Update `CONTRIBUTING.md` to recommend `make check` as the pre-PR command that matches CI; verify the command exists and the text references it

## 4. End-to-end verification

- [x] 4.1 On a clean tree, run the exact CI formatting gate `test -z "$(gofmt -l .)"` and confirm it exits 0
- [x] 4.2 Confirm `go vet ./...`, `go build ./...` (CGO_ENABLED=0) and `go test ./...` (CGO_ENABLED=0) all pass and that `.github/workflows/ci.yml` is unchanged
