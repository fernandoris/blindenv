## ADDED Requirements

### Requirement: Tool discover_secrets

The Tool SHALL return the NAMES of the secret keys available to the resolved project, grouped by scope, without returning any value. It SHALL include the shared global scope, the environment-global scope for every environment name that carries at least one such key, the project-global scope of the resolved project, and the project + environment scope for every environment of the project that defines one. The Tool MUST be independent of the resolved environment: it SHALL NOT hide environment-scoped keys when the resolved environment is empty or different, and it SHALL NOT fail because a shared environment name has no matching environment in the project.

#### Scenario: Names, scope and environment without values

- **WHEN** the model invokes `discover_secrets`
- **THEN** the response contains key names with their scope and, for environment-scoped keys, the environment name, and no value

#### Scenario: Environment-scoped keys visible regardless of resolved environment

- **WHEN** an environment-global key exists for environment `DES` and the resolved environment is empty or a different name
- **THEN** the response includes that key under scope `environment` with environment `DES`

#### Scenario: Shared environment without a project environment

- **WHEN** an environment-global key exists for environment `DES` and the resolved project has no environment named `DES`
- **THEN** the response lists that key instead of failing

#### Scenario: Grouped by scope

- **WHEN** the vault defines a global key, an environment-global key, a project-global key and a project + environment key
- **THEN** the response places each key under its corresponding scope group and environment

#### Scenario: Discovery never returns values

- **WHEN** the model invokes `discover_secrets`
- **THEN** the response contains only names, scopes and environment names, and no secret value

### Requirement: Discovery phase in server guidance

The server SHALL advertise a discovery-first workflow in its MCP instructions: the model discovers available keys with `discover_secrets`, then selects a project and environment and uses secrets through `proxy_http_request` or `execute_with_secrets`.

#### Scenario: Guidance advertises discovery

- **WHEN** an MCP client reads the server instructions
- **THEN** the instructions describe `discover_secrets` and the discover-then-use order before the secret-consuming tools

## MODIFIED Requirements

### Requirement: Context pinned in configuration with override

The server SHALL take the project and environment from the MCP client configuration when the Tool does not receive them, and SHALL allow overriding them per call when provided. The server MUST require a non-empty resolved environment for the secret-consuming tools (`proxy_http_request` and `execute_with_secrets`); a call that resolves to an empty environment SHALL be refused with an error explaining that an environment is required. The discovery and listing tools (`discover_secrets`, `list_secret_keys`, `get_context`) SHALL NOT require an environment.

#### Scenario: Using the pinned context

- **WHEN** the model invokes a Tool without specifying project or environment
- **THEN** the system uses the project and environment pinned in the configuration

#### Scenario: Per-call override

- **WHEN** the model invokes a Tool specifying an environment different from the pinned one
- **THEN** the system uses the environment given in the call

#### Scenario: Empty environment refused for secret-consuming tools

- **WHEN** the model invokes `proxy_http_request` or `execute_with_secrets` and neither the call nor the pinned configuration provides an environment
- **THEN** the system refuses the call with an error explaining that an environment is required, without performing the request or running any command

#### Scenario: Listing tools allow an empty environment

- **WHEN** the model invokes `discover_secrets`, `list_secret_keys` or `get_context` and no environment is provided
- **THEN** the system returns the available names without requiring an environment
