## 1. Store: grouped scope enumeration

- [x] 1.1 Add a store method that returns grouped key NAMES for a project: global, environment-global per shared environment, project-global, and project + environment per project environment, reusing `ScopeSecrets` / `ListSharedEnvironments` (`pkg/db/store.go`). Verify with a db test that lists a key for each of the four scope groups.
- [x] 1.2 Verify the method returns names only and never decrypts values, by asserting the returned data has no value field and no `crypto.Decrypt` call is exercised (test in `pkg/db/db_test.go`).
- [x] 1.3 Verify a shared environment key is listed for a project that has no environment of that name, and that a shared global key is listed for any project (test in `pkg/db/db_test.go`).

## 2. MCP tool discover_secrets

- [x] 2.1 Register `discover_secrets` with a description stating it returns names, scope and environment but no values, and implement the handler using `resolveContext` (project required, environment optional) plus the store method from section 1. Verify `TestDiscoverSecrets` in `pkg/mcp/server_test.go` returns the grouped shape.
- [x] 2.2 Verify discovery is independent of the resolved environment: with an environment-global key for `DES` and the resolved environment empty or a different name, the response still includes it under `environment` / `DES` (test in `pkg/mcp/server_test.go`).
- [x] 2.3 Verify no value ever appears in the response for a populated vault (test in `pkg/mcp/server_test.go`).
- [x] 2.4 Update the integration test tool list in `cmd/blindenv/integration_test.go` to include `discover_secrets` and verify it passes.

## 3. Explicit environment for secret-consuming tools

- [x] 3.1 In `handleExecute` (`pkg/mcp/exec.go`) and `handleProxy` (`pkg/mcp/proxy.go`) refuse a call whose resolved environment is empty, returning an error that explains an environment is required. Verify with tests that no subprocess starts and no HTTP request is issued in that case.
- [x] 3.2 Verify `discover_secrets`, `list_secret_keys` and `get_context` still succeed with an empty environment (tests in `pkg/mcp/server_test.go`).

## 4. Server guidance and documentation

- [x] 4.1 Update the server `instructions` (`pkg/mcp/server.go`) to describe `discover_secrets` and the discover-then-use order, and verify a test asserts the instructions mention `discover_secrets`.
- [x] 4.2 Update the README tool list and usage notes to include `discover_secrets` and the environment requirement for secret-consuming tools, and verify the mention is present.

## 5. End-to-end verification

- [x] 5.1 Run `go test ./...` and confirm the full suite passes.
- [x] 5.2 Run `openspec validate add-secret-discovery` and confirm the change is valid.
