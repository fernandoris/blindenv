## MODIFIED Requirements

### Requirement: Exact-match redaction

The system SHALL replace every literal occurrence of a sensitive secret value in captured command output (stdout and stderr) with a marker. Values whose effective definition is non-sensitive MUST NOT be redacted.

#### Scenario: Value present in stdout

- **WHEN** command output contains a sensitive secret value of the resolved project and environment
- **THEN** the system replaces it with a redaction marker before returning the response

#### Scenario: Value present in stderr

- **WHEN** command error output contains a sensitive secret value
- **THEN** the system replaces it with a redaction marker

#### Scenario: Value absent

- **WHEN** command output contains no sensitive secret value
- **THEN** the system returns the output unchanged

#### Scenario: Non-sensitive value left unchanged

- **WHEN** command output contains a non-sensitive configuration value
- **THEN** the system returns it unchanged
