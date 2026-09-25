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

The system SHALL normalize captured output to UTF-8 before applying redaction, covering common Windows encodings (e.g. PowerShell 5.1 UTF-16LE). Speculative BOM-less UTF-16 detection MUST be limited to the retention budget, so that a large non-UTF-16 buffer is never decoded as UTF-16 and amplified in memory.

#### Scenario: UTF-16 output

- **WHEN** a command on Windows produces UTF-16LE output containing a secret value
- **THEN** after normalization to UTF-8 the value is detected and redacted

#### Scenario: Large binary-like output is not speculatively decoded

- **WHEN** captured output exceeds the retention budget without a UTF-16 byte order mark
- **THEN** the system does not decode the whole buffer as UTF-16 and the memory used remains bounded

### Requirement: Bounded normalization and substitution budget

The system SHALL apply encoding normalization and value substitution only to the retained, already-capped output, so that the memory used by redaction is bounded by the retention maximum plus a constant and does not grow with the child's total output volume.

#### Scenario: Redaction over a bounded prefix

- **WHEN** a command emits far more output than the retention maximum
- **THEN** normalization and substitution run only over the retained prefix and peak memory stays bounded

#### Scenario: No per-key full-text duplication

- **WHEN** many keys are configured and the retained output is large
- **THEN** substitution does not require copying the entire retained text once per key beyond the configured bound

### Requirement: Redaction count reporting

The system SHALL report how many redaction substitutions were performed in the returned response.

#### Scenario: Redaction count

- **WHEN** the output contained the same value twice
- **THEN** the response reports a redaction count of two
