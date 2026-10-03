## MODIFIED Requirements

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

## ADDED Requirements

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
