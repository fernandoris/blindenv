# vault Specification

## Purpose

Stores dev/pre-prod secrets locally and encrypted so that plaintext values exist only in memory, organized in four scopes of increasing specificity: global and environment-global (shared across projects), project-global, and project + environment.

## Requirements

### Requirement: Master key management

The system SHALL obtain the master key (32 bytes) by preferring the operating system keyring and, when it is unavailable, by deriving it from a user-supplied passphrase using Argon2id. The master key MUST NOT ever be stored in plaintext next to the encrypted data.

#### Scenario: Keyring available

- **WHEN** a master key exists in the operating system keyring
- **THEN** the system uses it without prompting for a passphrase

#### Scenario: Environment without a keyring

- **WHEN** the system runs in an environment without a keyring (e.g. CI or container)
- **THEN** the system prompts for a passphrase and derives the master key with Argon2id using a persisted salt

#### Scenario: Master key missing without fallback

- **WHEN** there is no key in the keyring and no passphrase is provided
- **THEN** the system fails with an error explaining how to unlock the vault, without exposing data

### Requirement: Per-value encryption

The system SHALL encrypt each sensitive secret value independently with AES-256-GCM and a random nonce per operation. Project, environment and key names MUST remain readable. A value whose definition is non-sensitive MUST NOT be encrypted, SHALL be readable in storage, and SHALL be marked as non-sensitive; a sensitive value MUST NOT be readable in storage.

#### Scenario: Same value encrypted twice

- **WHEN** the same sensitive secret value is stored twice
- **THEN** the resulting ciphertexts are different

#### Scenario: Tamper detection

- **WHEN** the encrypted data of a sensitive value is modified on disk
- **THEN** decryption fails and the system reports an integrity error instead of returning an incorrect value

#### Scenario: Readable metadata

- **WHEN** the storage is inspected without decrypting
- **THEN** project, environment and key names are readable and sensitive values are not

#### Scenario: Configuration value readable at rest

- **WHEN** the storage is inspected without decrypting
- **THEN** a non-sensitive configuration value is readable

### Requirement: Global and per-environment secret model

The system SHALL allow global secrets (applying to every environment of a project) and per-environment secrets within the same project. When resolving the effective value of a key, the environment value SHALL take precedence over the global value.

#### Scenario: Global applies to all environments

- **WHEN** a key exists only as a project global
- **THEN** its value resolves in any environment of that project

#### Scenario: Environment overrides global

- **WHEN** a key exists both as a global and in a specific environment
- **THEN** the value resolved for that environment is the environment value

#### Scenario: Fallback to global

- **WHEN** a key exists as a global and does not exist in the queried environment
- **THEN** the resolved value is the global value

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

### Requirement: Weak secret warning

The system SHALL warn the user when a secret value is too short to be reliably redacted, without necessarily preventing storage.

#### Scenario: Short value

- **WHEN** the user stores a secret whose length is below the redaction threshold
- **THEN** the system shows a warning that the value will not be redacted from command output

### Requirement: Portable backup

The system SHALL allow exporting the vault to a file encrypted with a user-chosen passphrase and importing it on another machine, independently of the availability of the source keyring. The backup SHALL include secrets from all four scopes, and import SHALL accept backups produced before the shared scopes existed. The backup SHALL preserve each definition's type, hint and sensitivity, and SHALL accept backups produced before key metadata existed, importing their definitions as sensitive with no type and no hint.

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

#### Scenario: Metadata survives a round trip

- **WHEN** a vault containing typed, hinted and non-sensitive definitions is exported and imported
- **THEN** every definition is restored with its type, hint and sensitivity intact

#### Scenario: Import of a pre-metadata backup

- **WHEN** a backup created before key metadata existed is imported
- **THEN** its definitions are imported as sensitive with no type and no hint

### Requirement: Multi-process concurrency

The system SHALL support several processes (multiple MCP servers and the UI) reading and writing the same storage concurrently without corrupting it, including simultaneous access to shared scopes.

#### Scenario: Concurrent writes

- **WHEN** the UI writes a shared secret while an MCP server reads another project
- **THEN** both operations complete without corruption or unhandled lock errors

### Requirement: Secret key metadata

The system SHALL allow each secret definition to carry optional metadata: a type drawn from the closed set `url`, `host`, `connection-string`, `token`, `password`, `text`; a free-form hint describing how to use the value; and a sensitivity flag. The type and hint MUST describe the value and MUST NOT contain the value itself. When no metadata is supplied, a definition SHALL default to type `text`, sensitivity `true`, and an empty hint. Metadata SHALL be attached to an individual definition, so the same key defined in several scopes MAY carry different metadata.

#### Scenario: Metadata is per definition

- **WHEN** the same key is defined in two scopes with different types or hints
- **THEN** each definition keeps its own metadata

#### Scenario: Defaults when no metadata is supplied

- **WHEN** a value is stored without a type and without a sensitivity flag
- **THEN** the definition is type `text`, sensitive, with no hint

#### Scenario: Hint must not contain the value

- **WHEN** a hint is submitted that contains the definition's value
- **THEN** the system rejects the hint

#### Scenario: Unknown type rejected

- **WHEN** a definition is stored with a type outside the closed set
- **THEN** the system rejects the type

### Requirement: Configuration values

The system SHALL allow a definition to be marked non-sensitive. A non-sensitive value MUST be stored unencrypted, MUST NOT be redacted from captured output, and SHALL be readable by the agent-facing Tools. The type SHALL constrain sensitivity: `token` and `password` definitions MUST be sensitive, while `url` and `host` MAY default to non-sensitive.

#### Scenario: Non-sensitive value is not redacted

- **WHEN** a non-sensitive value appears in captured command output
- **THEN** the system leaves it unchanged

#### Scenario: Sensitive type cannot be non-sensitive

- **WHEN** a definition is submitted as type `token` or `password` and non-sensitive
- **THEN** the system rejects it

#### Scenario: Marking a value non-sensitive is explicit

- **WHEN** a value is stored without specifying sensitivity
- **THEN** it is stored as sensitive and encrypted

#### Scenario: Type may suggest a non-sensitive default

- **WHEN** a definition is submitted with type `url` and no sensitivity
- **THEN** the system MAY store it as non-sensitive, as long as the choice is explicit and visible to the user

### Requirement: Project browser-open capability

The vault SHALL store a per-project boolean capability `allow_open` that controls whether the MCP `open_in_browser` Tool may open a URL for that project. A newly created project SHALL default `allow_open` to disabled. The capability SHALL be readable and writable independently of `allow_execute`. Existing vaults SHALL be migrated so that every existing project has `allow_open` disabled.

#### Scenario: New project defaults disabled

- **WHEN** a project is created without specifying `allow_open`
- **THEN** its `allow_open` is disabled

#### Scenario: Capability is independent of execution

- **WHEN** `allow_execute` is enabled for a project and `allow_open` is not changed
- **THEN** `allow_open` remains in its previous state

#### Scenario: Existing projects migrate disabled

- **WHEN** a vault created before this capability existed is opened by a newer version
- **THEN** every existing project has `allow_open` disabled

### Requirement: Schema version compatibility

The vault SHALL persist a schema version and the system SHALL read it when opening the vault. When the stored version is greater than the version supported by the running binary, the system MUST refuse to open the vault, MUST NOT modify any part of it (including the stored version), and SHALL report an actionable error that states the vault's version, the highest supported version, and that the vault was not modified.

#### Scenario: Opening a newer vault is refused untouched

- **WHEN** a vault whose stored schema version is greater than the running binary supports is opened
- **THEN** the system fails with an error naming both versions and stating the vault was not modified
- **AND** the stored schema version and every table are unchanged

#### Scenario: Opening a current vault succeeds

- **WHEN** a vault whose stored schema version equals the version supported by the binary is opened
- **THEN** the system opens it normally without applying any migration

#### Scenario: Opening an older vault migrates it

- **WHEN** a vault whose stored schema version is lower than the version supported by the binary is opened
- **THEN** the system applies the pending migrations so the stored version becomes the supported version

### Requirement: Monotonic schema version

The stored schema version SHALL advance only. The system MUST NOT lower the stored version, including when a vault is opened by a binary whose supported version is lower than the vault's stored version.

#### Scenario: Refused open does not lower the marker

- **WHEN** a newer vault is opened by an older binary and the open is refused
- **THEN** the stored schema version remains the higher value

#### Scenario: Successful migration records the supported version

- **WHEN** migrations complete successfully
- **THEN** the stored schema version equals the version supported by the binary

### Requirement: Automatic pre-migration snapshot

Before applying any schema change to a vault that is older than the binary supports, the system SHALL create a consistent copy of the vault beside it, without requiring a passphrase, preserving the encrypted values exactly. The snapshot SHALL NOT be taken for a newly created vault nor for a vault already at the supported version, and an existing snapshot for the same source version MUST NOT be overwritten.

#### Scenario: Upgrade creates a restorable snapshot

- **WHEN** a vault older than the binary is migrated
- **THEN** a snapshot of the vault as it was before the migration is written beside it
- **AND** restoring that snapshot reproduces the pre-migration vault

#### Scenario: Fresh vault does not snapshot

- **WHEN** the vault is created for the first time
- **THEN** no snapshot is written

#### Scenario: Current vault does not snapshot

- **WHEN** the vault is already at the supported version
- **THEN** no snapshot is written

#### Scenario: Existing snapshot is preserved

- **WHEN** a snapshot for the same source version already exists before an upgrade
- **THEN** the existing snapshot is left unchanged

### Requirement: Unattended migration

Migration SHALL proceed without interactive input, so that it completes in a headless, non-interactive context.

#### Scenario: Headless upgrade

- **WHEN** a vault older than the binary is opened with no terminal attached
- **THEN** the migration completes without prompting for input

### Requirement: Bounded migration resources

Creating the pre-migration snapshot MUST NOT load the entire vault into process memory; the snapshot SHALL be produced by the storage engine so that peak memory does not grow with the size of the vault.

#### Scenario: Large vault snapshot memory is bounded

- **WHEN** a large vault is migrated
- **THEN** the memory allocated for the snapshot does not scale with the vault size

### Requirement: Forward-only migration with recovery

Migration SHALL be forward-only and the system SHALL NOT attempt an in-place downgrade. A pre-migration snapshot SHALL be a complete copy of the vault as it was before that upgrade, so an unwanted upgrade can be reversed by restoring it.

#### Scenario: Downgrade uses the snapshot

- **WHEN** a vault has been migrated and the previous behavior is required
- **THEN** restoring the pre-migration snapshot over the vault reproduces the pre-migration behavior without the system performing a reverse migration
