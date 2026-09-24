# vault Specification

## Purpose

Stores dev/pre-prod secrets locally and encrypted so that plaintext values exist only in memory, organized by project with global and per-environment secrets.

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

The system SHALL encrypt each secret value independently with AES-256-GCM and a random nonce per operation. Project, environment and key names MUST remain readable; values MUST NOT.

#### Scenario: Same value encrypted twice

- **WHEN** the same secret value is stored twice
- **THEN** the resulting ciphertexts are different

#### Scenario: Tamper detection

- **WHEN** the encrypted data of a value is modified on disk
- **THEN** decryption fails and the system reports an integrity error instead of returning an incorrect value

#### Scenario: Readable metadata

- **WHEN** the storage is inspected without decrypting
- **THEN** project, environment and key names are readable and values are not

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

### Requirement: Identifiers and uniqueness

The system SHALL identify each project by a unique name (slug). It SHALL allow the same key to exist as a global and as an environment secret, but MUST NOT allow duplicates within the same scope.

#### Scenario: Duplicate project

- **WHEN** creation of a project with an existing slug is attempted
- **THEN** the system rejects the creation

#### Scenario: Duplicate key in the same scope

- **WHEN** creation of a key that already exists in the same project and scope (global or same environment) is attempted
- **THEN** the system rejects the operation

#### Scenario: Global and environment with the same key

- **WHEN** a key is defined as global and another with the same name is defined in an environment of the same project
- **THEN** the system accepts both and applies the precedence rule

### Requirement: Weak secret warning

The system SHALL warn the user when a secret value is too short to be reliably redacted, without necessarily preventing storage.

#### Scenario: Short value

- **WHEN** the user stores a secret whose length is below the redaction threshold
- **THEN** the system shows a warning that the value will not be redacted from command output

### Requirement: Portable backup

The system SHALL allow exporting the vault to a file encrypted with a user-chosen passphrase and importing it on another machine, independently of the availability of the source keyring.

#### Scenario: Export and import on another machine

- **WHEN** the vault is exported with a passphrase and imported on another machine using that passphrase
- **THEN** projects, environments and secrets become available with their original values

#### Scenario: Wrong passphrase on import

- **WHEN** a backup is imported with an incorrect passphrase
- **THEN** the system rejects the import and does not modify the existing vault

### Requirement: Multi-process concurrency

The system SHALL support several processes (multiple MCP servers and the UI) reading and writing the same storage concurrently without corrupting it.

#### Scenario: Concurrent writes

- **WHEN** the UI writes a secret while an MCP server reads another
- **THEN** both operations complete without corruption or unhandled lock errors
