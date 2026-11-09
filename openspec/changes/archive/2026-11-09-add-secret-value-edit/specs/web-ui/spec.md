# Spec Delta

## ADDED Requirements

### Requirement: In-place value editing

The UI SHALL allow editing the value of an existing definition together with its metadata, without deleting and re-creating it and without revealing the current value. A blank replacement value SHALL keep the stored value. The edited definition SHALL keep its scope and key. The replacement field SHALL have an accessible label and SHALL state that leaving it blank keeps the current value.

#### Scenario: Edit value without revealing it

- **WHEN** the user edits a definition's value through the edit form
- **THEN** the new value is saved and the current value is never revealed

#### Scenario: Blank replacement keeps the value

- **WHEN** the user submits the edit form without entering a replacement value
- **THEN** the stored value is unchanged

#### Scenario: Value and metadata change together

- **WHEN** the user changes a definition's value and its type, sensitivity or hint in one save
- **THEN** all the changes are applied to the same definition

#### Scenario: Editing does not move the definition

- **WHEN** the user saves an edit
- **THEN** the definition keeps its scope and key and no definition is created elsewhere

#### Scenario: Removed definition cannot be edited

- **WHEN** the definition no longer exists in the selected scope at save time
- **THEN** the UI reports the failure inline and does not create a definition

#### Scenario: Short value warning after edit

- **WHEN** an edited value is shorter than the redaction threshold
- **THEN** the UI warns that the value will not be redacted from command output
