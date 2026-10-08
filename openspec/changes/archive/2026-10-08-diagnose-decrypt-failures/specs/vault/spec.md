## MODIFIED Requirements

### Requirement: Per-value encryption

The system SHALL encrypt each sensitive secret value independently with AES-256-GCM and a random nonce per operation. Project, environment and key names MUST remain readable. A value whose definition is non-sensitive MUST NOT be encrypted, SHALL be readable in storage, and SHALL be marked as non-sensitive; a sensitive value MUST NOT be readable in storage. A stored sensitive value SHALL always be a valid AES-256-GCM blob; a definition marked sensitive whose stored value cannot be a valid ciphertext is an inconsistent definition. A value whose effective definition is non-sensitive MUST NOT be passed to decryption.

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

#### Scenario: Non-sensitive value is never decrypted

- **WHEN** the effective definition of a key is non-sensitive
- **THEN** the system returns its stored cleartext without attempting decryption

## ADDED Requirements

### Requirement: Decryption failure classification

When resolving effective secrets, the system SHALL distinguish a master key that does not match the vault from a specific definition that cannot be read, and SHALL report each with an actionable error that contains no secret value. When the vault has at least one sensitive definition and none of its sensitive values decrypts with the provided master key, the system SHALL report a master-key mismatch and explain the likely causes (a different passphrase or keyring, or a vault written by another installation or version). When only some sensitive definitions fail, the system SHALL report the specific unreadable definition. A definition marked sensitive whose stored value is shorter than the AES-GCM nonce plus tag length SHALL be reported as an inconsistent definition rather than as a tampered value. When the vault has no sensitive definitions, resolution SHALL succeed for any master key.

#### Scenario: Master key does not match the vault

- **WHEN** the vault has at least one sensitive definition and no sensitive value decrypts with the provided master key
- **THEN** the system reports a master-key mismatch describing the likely causes, instead of a per-key integrity error

#### Scenario: A single definition is unreadable

- **WHEN** some sensitive definitions decrypt and one does not
- **THEN** the system reports that specific definition as unreadable, distinct from a master-key mismatch

#### Scenario: Structurally impossible ciphertext

- **WHEN** a definition is marked sensitive but its stored value is shorter than the nonce plus tag length
- **THEN** the system reports an inconsistent definition, distinct from a tampered value

#### Scenario: No sensitive definitions

- **WHEN** the vault has no sensitive definitions
- **THEN** resolution succeeds regardless of the master key

#### Scenario: Failure errors carry no values

- **WHEN** the system reports an unreadable vault or an unreadable definition
- **THEN** the error contains no secret value

### Requirement: Master key verification at unlock

When a vault that contains at least one sensitive definition is unlocked for use, the system SHALL verify that the provided master key can read the vault before serving any value, and SHALL fail with a master-key mismatch when no sensitive definition can be decrypted with it. The verification SHALL stop at the first sensitive definition that decrypts, so a matching key costs a single decryption. A vault with no sensitive definitions SHALL unlock without verification. When at least one sensitive definition decrypts, the vault SHALL unlock even if some other definitions cannot be read; those are reported per definition when resolved.

#### Scenario: Wrong master key fails at unlock

- **WHEN** a vault containing sensitive definitions is unlocked with a master key that decrypts none of them
- **THEN** unlocking fails with a master-key mismatch before any value is served

#### Scenario: Vault without sensitive definitions unlocks without a key check

- **WHEN** a vault containing no sensitive definitions is unlocked
- **THEN** it unlocks successfully without attempting decryption

#### Scenario: Partially unreadable vault still unlocks

- **WHEN** a vault is unlocked with a key that decrypts at least one but not all sensitive definitions
- **THEN** the vault unlocks and the unreadable definitions are reported individually when resolved
