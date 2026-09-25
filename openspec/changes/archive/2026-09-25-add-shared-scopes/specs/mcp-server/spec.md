## MODIFIED Requirements

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

### Requirement: Tool execute_with_secrets

The Tool SHALL run a local subprocess injecting the effective secrets of the resolved project and environment into its environment, capture its output, apply the redaction engine, and return the redacted output along with the exit code. The effective secrets SHALL include the shared scopes when they are not shadowed by a more specific scope.

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

### Requirement: Audit without values

The server SHALL log every Tool invocation with timestamp, project, environment, Tool name, NAMES of the keys used and the scope each key resolved from, command when applicable, exit code and redaction count. The log MUST NOT contain secret values.

#### Scenario: Execution logged

- **WHEN** the model uses a Tool that consumes secrets
- **THEN** the system appends an audit entry with the key names, their source scopes and without their values

#### Scenario: Shared source recorded

- **WHEN** a key used by a Tool resolved from a shared scope
- **THEN** the audit entry records that scope as the source of the key
