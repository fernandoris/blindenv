## MODIFIED Requirements

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

## ADDED Requirements

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
