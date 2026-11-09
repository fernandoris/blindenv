# Spec Delta

## ADDED Requirements

### Requirement: In-place definition update

The system SHALL update an existing secret definition in place. An update SHALL NOT change the definition's scope or key, and SHALL preserve its creation time while advancing its modification time. When the caller supplies a new value, the definition SHALL take that value; when the caller omits it, the stored value SHALL be preserved. A sensitive value SHALL be re-encrypted with a fresh nonce on every update.

#### Scenario: Value replacement preserves identity

- **WHEN** an existing definition's value is updated
- **THEN** the definition holds the new value
- **AND** its scope and key are unchanged
- **AND** its creation time is unchanged while its modification time advances

#### Scenario: Omitted value is preserved

- **WHEN** only metadata is updated on an existing definition
- **THEN** the stored value is unchanged

#### Scenario: Sensitive value is re-encrypted

- **WHEN** a sensitive definition is updated, even with the same plaintext
- **THEN** its stored ciphertext differs from the ciphertext before the update

#### Scenario: Non-sensitive value stays cleartext

- **WHEN** a non-sensitive definition's value is updated
- **THEN** the new value is readable at rest and is not encrypted

#### Scenario: Sensitivity change re-encodes the value

- **WHEN** an update flips a definition between sensitive and non-sensitive
- **THEN** the value is stored in the representation required by the new sensitivity

#### Scenario: Short value is reported

- **WHEN** an updated value is shorter than the redaction threshold
- **THEN** the system reports that the value will not be reliably redacted

### Requirement: Update requires an existing definition

An update SHALL target a definition that already exists in the requested scope. When no definition exists for the key in that scope, the system MUST fail the update as not found and MUST NOT create a definition.

#### Scenario: Missing definition fails

- **WHEN** a value is updated for a key that is not defined in the requested scope
- **THEN** the system reports the definition as not found
- **AND** no definition is created

#### Scenario: Broader scope is not edited

- **WHEN** a key resolves only from a broader scope and an update targets a narrower scope
- **THEN** the update fails as not found and the broader definition is unchanged

### Requirement: Hint validation on update

When an update supplies or preserves a hint, the system SHALL validate that hint against the effective updated value and MUST reject an update whose hint contains that value.

#### Scenario: Hint containing the new value is rejected

- **WHEN** an update supplies a hint that contains the new value
- **THEN** the system rejects the update and stores nothing

#### Scenario: Preserved hint collides with the new value

- **WHEN** an update changes only the value and the preserved hint contains the new value
- **THEN** the system rejects the update and stores nothing
