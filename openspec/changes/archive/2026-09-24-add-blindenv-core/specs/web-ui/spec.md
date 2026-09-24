## Purpose

Provides a local interface for a person to manage projects, environments and secrets, review the audit log, and export or import the vault without depending on the AI agent.

## ADDED Requirements

### Requirement: Restricted local exposure

The UI server SHALL listen only on the loopback interface and SHALL require a valid session token and an expected `Host` header for API requests. It MUST NOT expose the API on external interfaces.

#### Scenario: Request without token

- **WHEN** a request reaches the API without a valid token
- **THEN** the system rejects it

#### Scenario: Unexpected Host header

- **WHEN** a request arrives with an unexpected `Host` header
- **THEN** the system rejects it

#### Scenario: Network binding

- **WHEN** the UI starts
- **THEN** the server listens only on `127.0.0.1` and the port is configurable

### Requirement: Project, environment and secret management

The UI SHALL allow creating, listing, editing and deleting projects, environments and secrets, distinguishing global from environment-specific secrets and showing when an environment secret overrides a global one.

#### Scenario: Override marker

- **WHEN** an environment secret has the same name as a global
- **THEN** the UI indicates it visually as an override

#### Scenario: Secret creation

- **WHEN** the user creates a secret specifying project, scope (global or environment), name and value
- **THEN** the secret becomes available for resolution by the Tools

### Requirement: Explicit reveal of values

The UI MUST NOT display secret values in listings and SHALL require an explicit user action to reveal one.

#### Scenario: Listing without values

- **WHEN** the user opens the secrets view of an environment
- **THEN** only names and metadata are shown, not values

#### Scenario: Reveal on demand

- **WHEN** the user requests to reveal a specific secret
- **THEN** the system shows its value

### Requirement: Execution permission control

The UI SHALL allow enabling or disabling execution with secrets (`allow_execute`) per project.

#### Scenario: Permission change

- **WHEN** the user disables `allow_execute` for a project
- **THEN** invocations of `execute_with_secrets` on that project are rejected

### Requirement: Audit review

The UI SHALL display the audit log, including names of keys used and without exposing values.

#### Scenario: Audit view

- **WHEN** the user opens the audit view
- **THEN** the recorded entries are shown with their key names and without values

### Requirement: Vault export and import

The UI SHALL allow exporting the vault to a passphrase-encrypted backup and importing it, reporting the outcome.

#### Scenario: Export

- **WHEN** the user exports the vault providing a passphrase
- **THEN** an encrypted backup file is generated

#### Scenario: Failed import

- **WHEN** the user imports a backup with an incorrect passphrase
- **THEN** the system shows an error and does not alter the current vault
