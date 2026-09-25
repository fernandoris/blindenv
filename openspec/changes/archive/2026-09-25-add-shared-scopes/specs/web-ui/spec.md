## ADDED Requirements

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

### Requirement: Definition and resolution separated

The UI SHALL distinguish secrets defined in the selected scope from secrets inherited from a broader scope, and SHALL provide a read-only effective view that lists each resolved key with the scope that supplied its winning value.

#### Scenario: Inherited secrets are marked

- **WHEN** the selected environment inherits keys from broader scopes
- **THEN** those keys are shown separately from the keys defined in the selected scope and each names its source scope

#### Scenario: Effective view

- **WHEN** the user opens the effective view for a project and environment
- **THEN** the UI lists each effective key and the scope it resolves from, without values

### Requirement: Inheritance and override actions

The UI SHALL let the user turn an inherited key into a definition in the selected scope, and SHALL show when a secret defined in the selected scope shadows a broader scope.

#### Scenario: Override from an inherited row

- **WHEN** the user chooses to override an inherited key
- **THEN** the UI opens the add-secret form prefilled with that key in the selected scope

#### Scenario: Shadowing indicator

- **WHEN** a secret defined in the selected scope has the same name as a broader-scope secret
- **THEN** the UI indicates that it overrides the broader scope

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

## MODIFIED Requirements

### Requirement: Project, environment and secret management

The UI SHALL allow creating, listing, editing and deleting projects, environments and secrets in the global, environment-global, project-global and project + environment scopes. It SHALL present one selected scope editor at a time and SHALL distinguish secrets defined in that scope from secrets inherited from broader scopes, naming the broader scope each inherited secret comes from.

#### Scenario: Override marker

- **WHEN** a secret defined in the selected scope has the same name as one in a broader scope
- **THEN** the UI indicates it visually as an override

#### Scenario: Secret creation

- **WHEN** the user creates a secret specifying project or shared scope, name, value and, when applicable, environment
- **THEN** the secret becomes available for resolution by the Tools in every matching project and environment

#### Scenario: Inherited secret display

- **WHEN** the user opens a project + environment scope
- **THEN** the UI lists keys defined there separately from keys inherited from the project-global, environment-global and global scopes, each labeled with its source

### Requirement: Explicit reveal of values

The UI MUST NOT display secret values in listings and SHALL require an explicit user action to reveal one, qualified by the scope that defines it. A revealed value SHALL be re-masked after a short interval and SHALL offer a copy action.

#### Scenario: Listing without values

- **WHEN** the user opens the secrets view of any scope
- **THEN** only names and metadata are shown, not values

#### Scenario: Reveal on demand

- **WHEN** the user requests to reveal a specific secret in a specific scope
- **THEN** the system shows that scope's value and re-masks it after the interval

#### Scenario: Shared value reveal is explicit

- **WHEN** the user reveals a value defined in a shared scope
- **THEN** the UI makes clear that the value applies beyond the current project

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
