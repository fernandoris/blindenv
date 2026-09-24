# secret-redaction Specification

## Purpose

Prevents secret values from appearing in the text the MCP server returns to the model, replacing them with markers before output leaves BlindEnv.

## Requirements

### Requirement: Exact-match redaction

The system SHALL replace every literal occurrence of a secret value in captured command output (stdout and stderr) with a marker.

#### Scenario: Value present in stdout

- **WHEN** command output contains a secret value of the resolved project and environment
- **THEN** the system replaces it with a redaction marker before returning the response

#### Scenario: Value present in stderr

- **WHEN** command error output contains a secret value
- **THEN** the system replaces it with a redaction marker

#### Scenario: Value absent

- **WHEN** command output contains no secret value
- **THEN** the system returns the output unchanged

### Requirement: Marker format

The system SHALL use the format `[BLINDENV_REDACTED:<KEY_NAME>]`, including the name of the key whose value was replaced.

#### Scenario: Marker with key name

- **WHEN** the value of key `API_KEY` is redacted
- **THEN** the resulting text contains `[BLINDENV_REDACTED:API_KEY]`

### Requirement: Minimum length threshold

The system SHALL redact only values whose length reaches a configured minimum, so that short or common values do not corrupt the output.

#### Scenario: Value below threshold

- **WHEN** a secret value is shorter than the minimum redaction length
- **THEN** the system performs no substitutions based on that value

### Requirement: Length-ordered substitution

The system SHALL process values from longest to shortest to prevent a shorter value that is a prefix of a longer one from leaving remnants of the longer value unredacted.

#### Scenario: Overlapping values

- **WHEN** two secret values exist where one is a prefix of the other and both appear in the output
- **THEN** the resulting output contains no fragment of either value

### Requirement: Encoding normalization first

The system SHALL normalize captured output to UTF-8 before applying redaction, covering common Windows encodings (e.g. PowerShell 5.1 UTF-16LE).

#### Scenario: UTF-16 output

- **WHEN** a command on Windows produces UTF-16LE output containing a secret value
- **THEN** after normalization to UTF-8 the value is detected and redacted

### Requirement: Redaction count reporting

The system SHALL report how many redaction substitutions were performed in the returned response.

#### Scenario: Redaction count

- **WHEN** the output contained the same value twice
- **THEN** the response reports a redaction count of two
