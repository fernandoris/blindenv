## 1. MCP payload

- [x] 1.1 In `handleGetContext` (`pkg/mcp/tools.go`), add `allow_execute` to the response from the resolved project row; verify a server test asserts it is `false` for the default project.
- [x] 1.2 Replace the `Store.ListKeys` call in `handleGetContext` with `Store.ListSecrets` and emit each key as `{ key, scope, environment?, overrides? }`, omitting `environment` unless the scope is `environment` and `overrides` when empty; verify the response no longer contains bare key strings.

## 2. Tests

- [x] 2.1 Update `TestGetContextNoValues` (`pkg/mcp/server_test.go`) to decode the object shape and assert `allow_execute`; add a case with `SetAllowExecute(ctx, "my-api", true)` asserting the field flips to `true`.
- [x] 2.2 Add a test proving provenance: a global key reports `scope: "global"`, an environment-global key reports `scope: "environment"` with its environment name, and a project value shadowing a broader scope lists it in `overrides`; verify `go test ./pkg/mcp/...` passes.
- [x] 2.3 Update the `get_context` assertions in `cmd/blindenv/integration_test.go` to check `allow_execute` is present and that no secret value leaks.

## 3. Docs

- [x] 3.1 Update the README MCP bullets (`README.md:44`, `README.md:248`) to state that `get_context` reports execution capability and per-key source scope, never values.

## 4. Verification

- [x] 4.1 Run `gofmt -w .`, `go vet ./...`, `go test ./...` and `CGO_ENABLED=0 go build ./...`; verify all pass.
