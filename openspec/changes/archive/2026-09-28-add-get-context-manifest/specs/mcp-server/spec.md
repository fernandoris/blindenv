## MODIFIED Requirements

### Requirement: Tool get_context

The Tool SHALL return execution context without secrets: operating system, architecture, shell hint, active project and environment, whether execution with secrets is enabled for the resolved project, and the effective keys with the source scope that supplied each value. For each key it SHALL report one of the four scopes (`global`, `environment`, `project`, `project_environment`), the environment name when the source scope is environment-specific, and the broader scopes the definition shadows. The Tool MUST NOT return secret values.

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
