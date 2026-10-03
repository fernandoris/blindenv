## MODIFIED Requirements

### Requirement: Tool get_context

The Tool SHALL return execution context without secrets: operating system, architecture, shell hint, the resolved shell's secret reference, active project and environment, whether execution with secrets is enabled for the resolved project, and the effective keys with the source scope that supplied each value. The secret reference SHALL report the canonical tag `{{SECRET_NAME}}` and the native environment reference for that shell (`${env:NAME}` for PowerShell/pwsh, `%NAME%` for cmd, `${NAME}` for POSIX shells). The Tool SHALL accept an optional `shell` argument and SHALL default to the OS shell when it is not provided. For each key it SHALL report one of the four scopes (`global`, `environment`, `project`, `project_environment`), the environment name when the source scope is environment-specific, and the broader scopes the definition shadows. The Tool MUST NOT return secret values.

#### Scenario: Context without values

- **WHEN** the model invokes `get_context`
- **THEN** the response includes the operating system, the project and environment, and the effective keys, but no value

#### Scenario: Execution capability reported

- **WHEN** the model invokes `get_context` for a project whose `allow_execute` is disabled or enabled
- **THEN** the response reports the matching execution capability for that project, before the model attempts `execute_with_secrets`

#### Scenario: Shared key source reported

- **WHEN** a key's effective value resolves from the global scope or from the environment-global scope for the queried environment
- **THEN** the response reports that shared scope as the source, and includes the environment name when the source is environment-global

#### Scenario: Project-scoped key source reported

- **WHEN** a key is defined only in the project-global scope or in the project + environment scope
- **THEN** the response reports that project scope as the source, so the model can tell a project-local key from a shared one

#### Scenario: Shadowed scopes reported

- **WHEN** a key is defined in more than one applicable scope
- **THEN** the response reports the winning scope and lists the broader scopes the definition shadows

#### Scenario: Secret reference reported

- **WHEN** the model invokes `get_context` with a shell, or without one
- **THEN** the response reports the canonical tag `{{SECRET_NAME}}` and the native environment reference for that shell, defaulting to the OS shell

### Requirement: Tool execute_with_secrets

The Tool SHALL run a local subprocess injecting the effective secrets of the resolved project and environment into its environment, capture a bounded amount of its output, apply the redaction engine, and return the redacted output along with the exit code. The effective secrets SHALL include the shared scopes when they are not shadowed by a more specific scope. Before running, the Tool SHALL translate every `{{SECRET_NAME}}` tag that names an effective key into the target shell's native environment reference, so the value is read from the injected environment and never placed on the command line; a tag whose name is not an effective key SHALL pass through unchanged. When the target shell is unknown or empty, or a matching tag appears inside a single-quoted region where the shell would not expand the reference, the Tool SHALL refuse the call with an error explaining how to proceed, rather than passing the tag through or embedding the value. The memory used to capture output MUST NOT grow with the child's total output volume: once a fixed maximum is retained, further bytes MUST be discarded rather than stored. The Tool MUST return within its execution timeout even when the child, or any descendant that inherited its stdout/stderr, keeps those streams open. On completion or timeout the Tool MUST terminate the entire child process tree.

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

#### Scenario: Tag translated to the shell reference

- **WHEN** a command for a known shell contains `{{API_KEY}}` and `API_KEY` is an effective key
- **THEN** the subprocess receives the equivalent native reference (`${env:API_KEY}`, `%API_KEY%` or `${API_KEY}`) and the value is never placed on the command line

#### Scenario: Non-matching tag passes through

- **WHEN** a command contains a `{{...}}` tag whose name is not an effective key
- **THEN** the tag is left unchanged for the shell to interpret

#### Scenario: Tag inside single quotes refused

- **WHEN** a matching `{{API_KEY}}` appears inside a single-quoted region for a shell that does not expand references there
- **THEN** the Tool refuses with an error explaining to use the native reference instead, without running the command

#### Scenario: Unknown shell with a matching tag refused

- **WHEN** a command contains a matching tag and the resolved shell is unknown or empty
- **THEN** the Tool refuses with an error explaining that a shell is required to translate the tag, without running the command

### Requirement: Discovery phase in server guidance

The server SHALL advertise a discovery-first workflow in its MCP instructions: the model discovers available keys with `discover_secrets`, then selects a project and environment and reads the shell's secret reference from `get_context`, then uses secrets through `proxy_http_request` or `execute_with_secrets`.

#### Scenario: Guidance advertises discovery

- **WHEN** an MCP client reads the server instructions
- **THEN** the instructions describe `discover_secrets` and the discover-then-use order before the secret-consuming tools

#### Scenario: Guidance describes the reference step

- **WHEN** an MCP client reads the server instructions
- **THEN** the instructions describe reading the shell's secret reference from `get_context` between discovery and use
