## MODIFIED Requirements

### Requirement: Tool discover_secrets

The Tool SHALL return the NAMES of the secret keys available to the resolved project, grouped by scope, without returning any secret VALUE. It SHALL include the shared global scope, the environment-global scope for every environment name that carries at least one such key, the project-global scope of the resolved project, and the project + environment scope for every environment of the project that defines one. For every key it SHALL also report the type and, when present, the hint, and whether the definition is non-sensitive. The Tool MUST be independent of the resolved environment: it SHALL NOT hide environment-scoped keys when the resolved environment is empty or different, and it SHALL NOT fail because a shared environment name has no matching environment in the project.

#### Scenario: Names, scope and environment without values

- **WHEN** the model invokes `discover_secrets`
- **THEN** the response contains key names with their scope and, for environment-scoped keys, the environment name, and no secret value

#### Scenario: Metadata reported per key

- **WHEN** the model invokes `discover_secrets`
- **THEN** every key reports its type, its hint when present, and whether it is non-sensitive

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
- **THEN** the response contains only names, metadata, scopes and environment names, and no secret value, including for non-sensitive keys

### Requirement: Tool get_context

The Tool SHALL return the execution context: operating system, architecture, shell hint, the resolved shell's secret reference, active project and environment, whether execution with secrets is enabled for the resolved project, and the effective keys with the source scope that supplied each value. For each key it SHALL report its type, its hint when present, and whether it is non-sensitive, alongside the source-scope information. The secret reference SHALL report the canonical tag `{{SECRET_NAME}}` and the native environment reference for that shell (`${env:NAME}` for PowerShell/pwsh, `%NAME%` for cmd, `${NAME}` for POSIX shells). The Tool SHALL accept an optional `shell` argument and SHALL default to the OS shell when it is not provided. For each key it SHALL report one of the four scopes (`global`, `environment`, `project`, `project_environment`), the environment name when the source scope is environment-specific, and the broader scopes the definition shadows. The Tool SHALL return the value of a key only when that key's effective definition is non-sensitive; it MUST NOT return the value of a sensitive key.

#### Scenario: Context without values

- **WHEN** the model invokes `get_context` and every effective key is sensitive
- **THEN** the response includes the operating system, the project and environment, and the effective keys, but no value

#### Scenario: Metadata reported per key

- **WHEN** the model invokes `get_context`
- **THEN** every effective key reports its type, its hint when present, and whether it is non-sensitive

#### Scenario: Non-sensitive value returned

- **WHEN** an effective key's winning definition is non-sensitive
- **THEN** the response includes that key's value

#### Scenario: Sensitive value withheld

- **WHEN** an effective key's winning definition is sensitive
- **THEN** the response does not include that key's value

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

## ADDED Requirements

### Requirement: Non-sensitive configuration values in secret-consuming tools

The secret-consuming Tools SHALL inject a non-sensitive value into the child environment or substitute it in an HTTP request exactly as a sensitive value, but MUST NOT redact its literal occurrences from returned output. A `{{SECRET_NAME}}` tag that names a non-sensitive key SHALL still be substituted (proxy) or translated to the shell's native reference (execute).

#### Scenario: Non-sensitive value injected

- **WHEN** a non-sensitive key is effective and the model runs a command referencing it
- **THEN** the child process receives the value in its environment

#### Scenario: Non-sensitive value not redacted

- **WHEN** command output prints a non-sensitive value
- **THEN** the response returned to the model contains it unchanged

#### Scenario: Proxy substitutes a non-sensitive tag

- **WHEN** an HTTP header contains `{{ENDPOINT}}` and `ENDPOINT` is a non-sensitive key
- **THEN** the outgoing request carries the value and no error is raised
