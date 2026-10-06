## Why

The CI formatting gate (`test -z "$(gofmt -l .)"`) has been failing since commit
`e0ea30f`, which hand-aligned only the lower half of two composite literals
after adding a longer key (`Substitutions` / `substitutions`). The red check
blocks merges even though `go vet`, `go build` and `go test` all pass, and no
local command reproduces the gate, so the slip was never caught before push.

## What Changes

- Reformat `pkg/db/db_test.go` and `pkg/web/server.go` with `gofmt` (alignment
  only; no behavior change).
- Add a `Makefile` with a `fmt` target (rewrites in place) and a `check` target
  that runs the same `gofmt -l` gate CI uses, plus `go vet`, `go build` and
  `go test`, so contributors can reproduce the pipeline locally.
- Point to `make check` from `CONTRIBUTING.md`.
- **Out of scope**: editing `.github/workflows/ci.yml`. The `actions/checkout`
  and `actions/setup-go` Node 20 deprecation warnings are a separate concern.

## Capabilities

### New Capabilities
<!-- None: this is a formatting fix plus developer tooling, with no spec-level
     behavior change. The change opts out of specs via skip_specs: true. -->

None.

### Modified Capabilities

None.

## Impact

- `pkg/db/db_test.go`, `pkg/web/server.go` - whitespace/alignment only.
- New `Makefile` (`fmt`, `check` targets).
- `CONTRIBUTING.md` - document the local check command.
- `.github/workflows/ci.yml` - unchanged.
- `.openspec.yaml` - sets `skip_specs: true` (no spec-level behavior change).
