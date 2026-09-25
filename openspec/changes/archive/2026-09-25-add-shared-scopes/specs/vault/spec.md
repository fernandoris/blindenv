## ADDED Requirements

### Requirement: Shared secret scopes

The system SHALL allow secrets to be defined outside any single project, in two shared scopes: an environment-global scope that applies to the named environment across every project, and a global scope that applies to every project and every environment. The system SHALL derive the set of shared environments from the names of existing project environments and from names already carrying an environment-global secret.

#### Scenario: Environment-global applies across projects

- **WHEN** a key is defined in the environment-global scope for environment "staging"
- **THEN** its value resolves for the "staging" environment of every project that has a "staging" environment

#### Scenario: Environment-global does not leak to other environment names

- **WHEN** a key is defined in the environment-global scope for environment "staging"
- **THEN** it does not resolve for a project environment named "prod"

#### Scenario: Global applies everywhere

- **WHEN** a key is defined in the global scope
- **THEN** its value resolves for every project and every environment

#### Scenario: Shared environment is offered before any project defines it

- **WHEN** an environment-global secret exists for a name no project currently uses
- **THEN** the system still treats that name as a shared environment

### Requirement: Effective resolution across scopes

The system SHALL resolve the effective value of a key by choosing the most specific scope in which the key is defined, in the order: project + environment, then project-global, then environment-global, then global. When the project-global and environment-global scopes both define the key, the project-global value SHALL win.

#### Scenario: Project and environment beats every other scope

- **WHEN** a key is defined in the project + environment scope and in one or more broader scopes
- **THEN** the value resolved for that project and environment is the project + environment value

#### Scenario: Project-global beats environment-global

- **WHEN** a key is defined both in the project-global scope and in the environment-global scope for the queried environment
- **THEN** the value resolved is the project-global value

#### Scenario: Environment-global beats global

- **WHEN** a key is defined both in the environment-global scope for the queried environment and in the global scope
- **THEN** the value resolved is the environment-global value

#### Scenario: Fallback through every tier

- **WHEN** a key is defined only in the global scope
- **THEN** its value resolves for every project and environment that does not define it more specifically

#### Scenario: Source scope can be determined

- **WHEN** the system resolves a key for a project and environment
- **THEN** it can report which of the four scopes supplied the winning value

### Requirement: Shared scope management

The system SHALL allow creating, updating and deleting secrets in the environment-global and global scopes, and SHALL allow the same key to be defined independently in each of the four scopes without collision.

#### Scenario: Same key in all scopes

- **WHEN** the same key is defined as global, environment-global, project-global and project + environment
- **THEN** all four definitions are stored and the precedence rule selects the effective value

#### Scenario: Delete a shared secret

- **WHEN** a secret is deleted from the global scope
- **THEN** keys of that name defined in more specific scopes remain and resolution falls back to them

#### Scenario: Duplicate within a shared scope

- **WHEN** a key is created twice in the same shared scope for the same environment name
- **THEN** the system updates the existing definition instead of creating a duplicate

## MODIFIED Requirements

### Requirement: Identifiers and uniqueness

The system SHALL identify each project by a unique name (slug). It SHALL allow the same key to exist simultaneously in each of the four scopes (global, environment-global, project-global and project + environment), but MUST NOT allow duplicates within the same scope. Environment sharing SHALL be matched by the environment name exactly.

#### Scenario: Duplicate project

- **WHEN** creation of a project with an existing slug is attempted
- **THEN** the system rejects the creation

#### Scenario: Duplicate key in the same scope

- **WHEN** creation of a key that already exists in the same project and scope is attempted
- **THEN** the system updates the existing value rather than creating a duplicate

#### Scenario: Global and environment with the same key

- **WHEN** a key is defined as project-global and another with the same name is defined in an environment of the same project
- **THEN** the system accepts both and applies the precedence rule

#### Scenario: Shared scope duplicate resolution

- **WHEN** a key is defined twice in the environment-global scope for the same environment name
- **THEN** the system stores a single definition and rejects the duplicate as a conflict

### Requirement: Portable backup

The system SHALL allow exporting the vault to a file encrypted with a user-chosen passphrase and importing it on another machine, independently of the availability of the source keyring. The backup SHALL include secrets from all four scopes, and import SHALL accept backups produced before the shared scopes existed.

#### Scenario: Export and import on another machine

- **WHEN** the vault is exported with a passphrase and imported on another machine using that passphrase
- **THEN** projects, environments, project-scoped secrets and shared secrets become available with their original values

#### Scenario: Shared secrets survive a round trip

- **WHEN** a vault containing environment-global and global secrets is exported and imported
- **THEN** those shared secrets are restored with their scopes intact

#### Scenario: Import of a pre-shared-scopes backup

- **WHEN** a backup created before shared scopes existed is imported
- **THEN** its project and environment secrets are restored into the corresponding project + environment and project-global scopes

#### Scenario: Wrong passphrase on import

- **WHEN** a backup is imported with an incorrect passphrase
- **THEN** the system rejects the import and does not modify the existing vault

### Requirement: Multi-process concurrency

The system SHALL support several processes (multiple MCP servers and the UI) reading and writing the same storage concurrently without corrupting it, including simultaneous access to shared scopes.

#### Scenario: Concurrent writes

- **WHEN** the UI writes a shared secret while an MCP server reads another project
- **THEN** both operations complete without corruption or unhandled lock errors
