# web-ui Specification

## Purpose

Provides a local interface for a person to manage projects, environments and secrets, review the audit log, and export or import the vault without depending on the AI agent.

## Requirements

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

### Requirement: Scope navigation

The UI SHALL present a scope navigation region with a Shared section (the global scope and each shared environment) and a Projects section (each project, its project-global scope and its environments). It SHALL open exactly one scope editor at a time, allow projects to be collapsed, show a count of secrets owned by each scope, and offer filtering by project and environment.

#### Scenario: One editor at a time

- **WHEN** the user selects a scope in the navigation
- **THEN** only that scope's editor is shown

#### Scenario: Collapsing projects

- **WHEN** a project holds many environments
- **THEN** the user can collapse it in the navigation while its secret count remains visible

#### Scenario: Shared scopes are reachable without a project

- **WHEN** the user wants to define a value for every project
- **THEN** the Shared section is reachable without selecting a project first

### Requirement: Project, environment and secret management

The UI SHALL allow creating, listing, editing and deleting projects, environments and secrets in the global, environment-global, project-global and project + environment scopes. It SHALL present one selected scope editor at a time and SHALL distinguish secrets defined in that scope from secrets inherited from broader scopes, naming the broader scope each inherited secret comes from. When creating or editing a secret, the UI SHALL allow entering or changing its type, its sensitivity and its hint.

#### Scenario: Override marker

- **WHEN** a secret defined in the selected scope has the same name as one in a broader scope
- **THEN** the UI indicates it visually as an override

#### Scenario: Secret creation

- **WHEN** the user creates a secret specifying project or shared scope, name, value and, when applicable, environment
- **THEN** the secret becomes available for resolution by the Tools in every matching project and environment

#### Scenario: Secret creation with metadata

- **WHEN** the user creates a secret and provides a type, a sensitivity or a hint
- **THEN** the definition is stored with that metadata

#### Scenario: Inherited secret display

- **WHEN** the user opens a project + environment scope
- **THEN** the UI lists keys defined there separately from keys inherited from the project-global, environment-global and global scopes, each labeled with its source

### Requirement: Definition and resolution separated

The UI SHALL distinguish secrets defined in the selected scope from secrets inherited from a broader scope, and SHALL provide a read-only effective view that lists each resolved key with the scope that supplied its winning value and its type. The effective view SHALL show the value of a non-sensitive key and MUST NOT show the value of a sensitive key without an explicit reveal.

#### Scenario: Inherited secrets are marked

- **WHEN** the selected environment inherits keys from broader scopes
- **THEN** those keys are shown separately from the keys defined in the selected scope and each names its source scope

#### Scenario: Effective view

- **WHEN** the user opens the effective view for a project and environment
- **THEN** the UI lists each effective key, its type and the scope it resolves from, showing values only for non-sensitive keys

### Requirement: Inheritance and override actions

The UI SHALL let the user turn an inherited key into a definition in the selected scope, and SHALL show when a secret defined in the selected scope shadows a broader scope.

#### Scenario: Override from an inherited row

- **WHEN** the user chooses to override an inherited key
- **THEN** the UI opens the add-secret form prefilled with that key in the selected scope

#### Scenario: Shadowing indicator

- **WHEN** a secret defined in the selected scope has the same name as a broader-scope secret
- **THEN** the UI indicates that it overrides the broader scope

### Requirement: Explicit reveal of values

The UI MUST NOT display the values of sensitive secrets in listings and SHALL require an explicit user action to reveal one, qualified by the scope that defines it. A revealed sensitive value SHALL be re-masked after a short interval and SHALL offer a copy action. The UI SHALL display the value of a non-sensitive definition without requiring a reveal and MUST NOT auto-hide it.

#### Scenario: Listing without values

- **WHEN** the user opens the secrets view of any scope
- **THEN** only names, metadata and non-sensitive values are shown, and sensitive values are masked

#### Scenario: Reveal on demand

- **WHEN** the user requests to reveal a specific sensitive secret in a specific scope
- **THEN** the system shows that scope's value and re-masks it after the interval

#### Scenario: Shared value reveal is explicit

- **WHEN** the user reveals a sensitive value defined in a shared scope
- **THEN** the UI makes clear that the value applies beyond the current project

#### Scenario: Non-sensitive value is visible without reveal

- **WHEN** a definition is non-sensitive
- **THEN** the UI shows its value without a reveal action and does not re-mask it

### Requirement: Destructive action safeguards

Before deleting a secret from a shared scope or a project-global scope, the UI SHALL show which projects and environments are affected and how they fall back, and SHALL require typed confirmation of the key name. Deleting a project or an environment SHALL report the keys that are removed with it.

#### Scenario: Shared delete impact

- **WHEN** the user attempts to delete a shared secret
- **THEN** the UI lists the affected scopes before requiring typed confirmation

#### Scenario: Overridden keys survive

- **WHEN** the user deletes a shared secret while a project already overrides the same key
- **THEN** the impact report states that the overriding scope is unaffected

#### Scenario: Environment delete

- **WHEN** the user deletes an environment
- **THEN** the UI reports the keys that will be removed

### Requirement: Feedback and accessibility

The UI SHALL report successes and errors inline instead of blocking alerts, SHALL warn when a stored value is too short to be redacted, and SHALL expose navigation state, disclosure state and revealed state with appropriate accessible attributes without relying on color alone.

#### Scenario: Short value warning

- **WHEN** the user stores a value shorter than the redaction threshold
- **THEN** the UI shows a warning that the value will not be redacted from command output

#### Scenario: Errors without blocking dialogs

- **WHEN** an operation fails
- **THEN** the UI shows a non-blocking message identifying the failure

#### Scenario: Screen reader state

- **WHEN** the user navigates scopes or reveals a value
- **THEN** the current scope, expanded state and revealed state are exposed to assistive technology

### Requirement: Execution permission control

The UI SHALL allow enabling or disabling execution with secrets (`allow_execute`) per project.

#### Scenario: Permission change

- **WHEN** the user disables `allow_execute` for a project
- **THEN** invocations of `execute_with_secrets` on that project are rejected

### Requirement: Audit review

The UI SHALL display the audit log from a slide-over panel opened on demand, including the names of keys used and the scope each resolved from, without exposing values. It SHALL allow filtering the entries by project, environment and Tool.

#### Scenario: Audit view

- **WHEN** the user opens the audit panel
- **THEN** the recorded entries are shown with their key names, source scopes and without values

#### Scenario: Audit does not occupy the main view

- **WHEN** the dashboard first loads
- **THEN** the audit log is closed and the primary editing surface is visible

#### Scenario: Filtering audit entries

- **WHEN** the user filters by project or environment
- **THEN** only matching audit entries are shown

### Requirement: Vault export and import

The UI SHALL allow exporting the vault to a passphrase-encrypted backup and importing it, reporting the outcome.

#### Scenario: Export

- **WHEN** the user exports the vault providing a passphrase
- **THEN** an encrypted backup file is generated

#### Scenario: Failed import

- **WHEN** the user imports a backup with an incorrect passphrase
- **THEN** the system shows an error and does not alter the current vault

### Requirement: Key metadata capture and editing

The UI SHALL capture a type, a sensitivity and an optional hint when a secret is created, and SHALL allow editing those metadata for an existing definition without requiring the user to re-enter the value. It SHALL prevent a sensitivity choice that contradicts the type (`token` and `password` are always sensitive) and SHALL warn that a non-sensitive value is stored unencrypted and visible to agents.

#### Scenario: Edit metadata without re-entering the value

- **WHEN** the user edits the type, sensitivity or hint of an existing definition
- **THEN** the change is saved without the value being re-entered or revealed

#### Scenario: Contradictory sensitivity prevented

- **WHEN** the user selects type `token` or `password`
- **THEN** the non-sensitive option is unavailable

#### Scenario: Unencrypted storage warning

- **WHEN** the user marks a definition non-sensitive
- **THEN** the UI warns that the value is stored unencrypted and is visible to agents

### Requirement: Key metadata presentation

The UI SHALL present each key's type and hint in the defined, inherited and effective listings, and SHALL expose the hint to assistive technology with an expandable control rather than a hover-only tooltip. It SHALL mark a non-sensitive definition distinctly and MUST NOT rely on color alone to distinguish sensitive from non-sensitive.

#### Scenario: Type and hint shown per row

- **WHEN** a key has a type and a hint
- **THEN** the listing shows the type and the hint for that key

#### Scenario: Hint reachable by keyboard

- **WHEN** the user navigates to a key's hint with the keyboard
- **THEN** the full hint can be expanded and read

#### Scenario: Non-sensitive marker is not color-only

- **WHEN** a definition is non-sensitive
- **THEN** the UI conveys that through text or an icon in addition to color

### Requirement: Metadata filtering

The UI filter SHALL match key names, types and hints, in addition to scope and environment.

#### Scenario: Filter matches a hint

- **WHEN** the user types text that appears only in a key's hint
- **THEN** the matching keys are shown

#### Scenario: Filter matches a type

- **WHEN** the user types a type name such as `url`
- **THEN** the keys of that type are shown

### Requirement: Browser-open permission control

The UI SHALL allow enabling or disabling opening URLs in the browser (`allow_open`) per project, independently of execution with secrets (`allow_execute`), and SHALL make clear that enabling it lets an agent hand a resolved URL to the local default browser.

#### Scenario: Permission change

- **WHEN** the user disables `allow_open` for a project
- **THEN** invocations of `open_in_browser` on that project are rejected

#### Scenario: Independent from execution

- **WHEN** the user toggles `allow_open`
- **THEN** the `allow_execute` state is unchanged
