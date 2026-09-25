## ADDED Requirements

### Requirement: Bounded normalization and substitution budget

The system SHALL apply encoding normalization and value substitution only to the retained, already-capped output, so that the memory used by redaction is bounded by the retention maximum plus a constant and does not grow with the child's total output volume.

#### Scenario: Redaction over a bounded prefix

- **WHEN** a command emits far more output than the retention maximum
- **THEN** normalization and substitution run only over the retained prefix and peak memory stays bounded

#### Scenario: No per-key full-text duplication

- **WHEN** many keys are configured and the retained output is large
- **THEN** substitution does not require copying the entire retained text once per key beyond the configured bound

## MODIFIED Requirements

### Requirement: Encoding normalization first

The system SHALL normalize captured output to UTF-8 before applying redaction, covering common Windows encodings (e.g. PowerShell 5.1 UTF-16LE). Speculative BOM-less UTF-16 detection MUST be limited to the retention budget, so that a large non-UTF-16 buffer is never decoded as UTF-16 and amplified in memory.

#### Scenario: UTF-16 output

- **WHEN** a command on Windows produces UTF-16LE output containing a secret value
- **THEN** after normalization to UTF-8 the value is detected and redacted

#### Scenario: Large binary-like output is not speculatively decoded

- **WHEN** captured output exceeds the retention budget without a UTF-16 byte order mark
- **THEN** the system does not decode the whole buffer as UTF-16 and the memory used remains bounded
