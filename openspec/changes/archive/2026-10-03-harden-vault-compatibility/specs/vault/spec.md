## ADDED Requirements

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
