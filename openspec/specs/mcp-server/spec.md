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

The Tool SHALL return only the NAMES of the effective keys for the resolved project and environment, drawn from all applicable scopes (project + environment, project-global, environment-global and global) without duplicates, never their values.

#### Scenario: Names without values

- **WHEN** the model invokes `list_secret_keys`
- **THEN** the response contains only key names and no value

#### Scenario: Union of global and environment

- **WHEN** the vault defines a global key, an environment-global key for the queried environment, a project-global key and a project + environment key
- **THEN** the response includes all four names, each once

#### Scenario: Shared keys appear in every matching project

- **WHEN** a global key is defined and the model queries any project and environment
- **THEN** the response includes that key name

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

The Tool SHALL run a local subprocess injecting the effective secrets of the resolved project and environment into its environment, capture a bounded amount of its output, apply the redaction engine, and return the redacted output along with the exit code. The effective secrets SHALL include the shared scopes when they are not shadowed by a more specific scope. The memory used to capture output MUST NOT grow with the child's total output volume: once a fixed maximum is retained, further bytes MUST be discarded rather than stored. The Tool MUST return within its execution timeout even when the child, or any descendant that inherited its stdout/stderr, keeps those streams open. On completion or timeout the Tool MUST terminate the entire child process tree.

#### Scenario: Project without execution permission

- **WHEN** the model invokes `execute_with_secrets` on a project with `allow_execute` disabled
- **THEN** the system refuses execution without running any subprocess

#### Scenario: Execution with injected secrets

- **WHEN** `allow_execute` is enabled for the project
- **THEN** the subprocess receives the effective secrets in its environment and its exit code is returned to the model

#### Scenario: Shared secret injected

- **WHEN** a key is defined only in a shared scope and the model runs a command
- **THEN** the subprocess receives that value in its environment

#### Scenario: Redacted output

- **WHEN** the subprocess prints a secret value in its output
- **THEN** the response returned to the model contains the redaction marker instead of the value

#### Scenario: Execution without a shell

- **WHEN** the model invokes the Tool with a separate command and arguments
- **THEN** the system runs the command directly without interpreting it through a shell

#### Scenario: Output larger than the retained maximum

- **WHEN** a command emits far more output than the retention maximum
- **THEN** the process memory used stays bounded, the response contains the retained prefix with a truncation indicator, and the exit code is reported

#### Scenario: Descendant keeps the stream open

- **WHEN** a command spawns a background descendant that inherits stdout/stderr and the direct child exits
- **THEN** the Tool terminates the process tree and returns within the execution timeout

#### Scenario: Command exceeds the timeout

- **WHEN** a command keeps running beyond the execution timeout
- **THEN** the Tool returns a timeout error within the deadline and the child process tree is terminated

### Requirement: Context pinned in configuration with override

The server SHALL take the project and environment from the MCP client configuration when the Tool does not receive them, and SHALL allow overriding them per call when provided.

#### Scenario: Using the pinned context

- **WHEN** the model invokes a Tool without specifying project or environment
- **THEN** the system uses the project and environment pinned in the configuration

#### Scenario: Per-call override

- **WHEN** the model invokes a Tool specifying an environment different from the pinned one
- **THEN** the system uses the environment given in the call

### Requirement: Audit without values

The server SHALL log every Tool invocation with timestamp, project, environment, Tool name, NAMES of the keys used and the scope each key resolved from, command when applicable, exit code and redaction count. The log MUST NOT contain secret values.

#### Scenario: Execution logged

- **WHEN** the model uses a Tool that consumes secrets
- **THEN** the system appends an audit entry with the key names, their source scopes and without their values

#### Scenario: Shared source recorded

- **WHEN** a key used by a Tool resolved from a shared scope
- **THEN** the audit entry records that scope as the source of the key

### Requirement: Errors without values

Error messages returned to the model MUST NOT contain secret values.

#### Scenario: Command failure

- **WHEN** a command fails and its error output contained a secret value
- **THEN** the error returned to the model contains the redaction marker instead of the value
