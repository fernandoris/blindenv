## Why

`get_context` reports the active project, environment and key names, but not the two facts an agent needs to plan: whether execution with secrets is even enabled, and where each key resolves from. The agent currently discovers `allow_execute` only by calling `execute_with_secrets` and hitting an error, and it cannot judge a key's availability reach because a shared key (global or environment-global) looks identical to one defined only in the current project. Both gaps force the agent to probe by failure instead of planning against a truthful manifest.

## What Changes

- Add a capability field to `get_context`: `allow_execute` as a boolean, so the agent can see the precondition before calling `execute_with_secrets` (the tool still refuses if the flag is off, since state can change between calls).
- Add per-key provenance to `get_context`: each key carries the scope that supplied its effective value (`global`, `environment`, `project`, `project_environment`), the environment name when the scope is environment-specific, and the broader scopes it shadows. This lets the agent reason about availability reach — a `global` key resolves everywhere, an `environment` key only for that environment name, while `project` and `project_environment` are confined to the current project.
- **BREAKING** (MCP response shape): `get_context`'s `secret_keys` changes from a list of strings to a list of objects. `list_secret_keys` is unchanged.
- Reuse the existing provenance computation (`Store.ListSecrets` / `SecretInfo`) rather than adding storage or resolution behavior.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `mcp-server`: the `get_context` Tool requirement gains the execution-capability flag and per-key source scope; new scenarios cover execution availability and shared-key provenance.

## Impact

- **MCP** (`pkg/mcp/tools.go`): `handleGetContext` reads the project row for `allow_execute` and resolves key provenance; its JSON payload changes shape.
- **Tests** (`pkg/mcp/server_test.go`, `cmd/blindenv/integration_test.go`): assert the new fields and the absence of values.
- **Docs** (`README.md`): note that `get_context` reports execution capability and key provenance.
- **No storage, no resolution, no execution behavior changes**: `proxy_http_request` and `execute_with_secrets` are untouched.
