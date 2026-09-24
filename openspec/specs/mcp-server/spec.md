# mcp-server Specification

## Purpose

Exposes the vault to AI agents through the Model Context Protocol, letting them use and reference dev/pre-prod secrets without the values entering the model context window.

## Requirements

### Requirement: stdio transport with reserved stdout

The MCP server SHALL communicate over `stdio` using JSON-RPC. The process MUST NOT write anything to stdout outside the protocol stream; logs MUST go to stderr or the audit log.

#### Scenario: Startup without corrupting the protocol

- **WHEN** the MCP server starts and emits log messages
- **THEN** logs are written to stderr and stdout contains only MCP protocol messages

### Requirement: Tool list_secret_keys

The Tool SHALL return only the NAMES of available keys for the resolved project and environment (union of global and environment keys, without duplicates), never their values.

#### Scenario: Names without values

- **WHEN** the model invokes `list_secret_keys`
- **THEN** the response contains only key names and no value

#### Scenario: Union of global and environment

- **WHEN** the project has a global key `REGION` and an environment key `API_KEY`
- **THEN** the response includes both names

### Requirement: Tool get_context

The Tool SHALL return execution context without secrets: operating system, architecture, shell hint, active project and environment, and the names of available keys.

#### Scenario: Context without values

- **WHEN** the model invokes `get_context`
- **THEN** the response includes the operating system, the project and environment, and the key names, but no value

### Requirement: Tool proxy_http_request

The Tool SHALL perform HTTP requests substituting `{{SECRET_NAME}}` tags in URL, headers and body inside BlindEnv, using the secrets of the resolved project and environment.

#### Scenario: Substitution in a header

- **WHEN** a header contains `{{API_KEY}}`
- **THEN** the outgoing request sends the real value and the model never receives it

#### Scenario: JSON body with quotes

- **WHEN** a secret value contains special characters and is substituted into a JSON body
- **THEN** the resulting body remains valid JSON

#### Scenario: Cross-host redirect

- **WHEN** a response redirects to a different host
- **THEN** the system does not forward secret-bearing headers to the new host

### Requirement: Tool execute_with_secrets

The Tool SHALL run a local subprocess injecting the secrets of the resolved project and environment into its environment, capture its output, apply the redaction engine, and return the redacted output along with the exit code.

#### Scenario: Project without execution permission

- **WHEN** the model invokes `execute_with_secrets` on a project with `allow_execute` disabled
- **THEN** the system refuses execution without running any subprocess

#### Scenario: Execution with injected secrets

- **WHEN** `allow_execute` is enabled for the project
- **THEN** the subprocess receives the secrets in its environment and its exit code is returned to the model

#### Scenario: Redacted output

- **WHEN** the subprocess prints a secret value in its output
- **THEN** the response returned to the model contains the redaction marker instead of the value

#### Scenario: Execution without a shell

- **WHEN** the model invokes the Tool with a separate command and arguments
- **THEN** the system runs the command directly without interpreting it through a shell

### Requirement: Context pinned in configuration with override

The server SHALL take the project and environment from the MCP client configuration when the Tool does not receive them, and SHALL allow overriding them per call when provided.

#### Scenario: Using the pinned context

- **WHEN** the model invokes a Tool without specifying project or environment
- **THEN** the system uses the project and environment pinned in the configuration

#### Scenario: Per-call override

- **WHEN** the model invokes a Tool specifying an environment different from the pinned one
- **THEN** the system uses the environment given in the call

### Requirement: Audit without values

The server SHALL log every Tool invocation with timestamp, project, environment, Tool name, NAMES of keys used, command when applicable, exit code and redaction count. The log MUST NOT contain secret values.

#### Scenario: Execution logged

- **WHEN** the model uses a Tool that consumes secrets
- **THEN** the system appends an audit entry with the key names and without their values

### Requirement: Errors without values

Error messages returned to the model MUST NOT contain secret values.

#### Scenario: Command failure

- **WHEN** a command fails and its error output contained a secret value
- **THEN** the error returned to the model contains the redaction marker instead of the value
