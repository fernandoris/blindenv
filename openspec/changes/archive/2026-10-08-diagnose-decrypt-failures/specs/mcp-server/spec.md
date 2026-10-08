## ADDED Requirements

### Requirement: Classified vault errors in secret-consuming Tools

When the unlocked vault's master key does not match (no sensitive definition decrypts), the value-consuming Tools (`get_context`, `proxy_http_request`, `execute_with_secrets`, `open_in_browser`) SHALL return an error stating the classified cause and MUST NOT contain any secret value. The discovery and listing Tools (`discover_secrets`, `list_secret_keys`) SHALL continue to return names and metadata, because they do not read values. When the master key matches but only a specific definition is unreadable or structurally inconsistent, the value-consuming Tools SHALL return that definition's error.

#### Scenario: Key mismatch surfaced by value-consuming Tools

- **WHEN** the vault was unlocked with a non-matching master key and the model invokes `execute_with_secrets` or `get_context`
- **THEN** the Tool returns a master-key-mismatch error explaining the likely causes, without running any command

#### Scenario: Discovery continues under a key mismatch

- **WHEN** the vault's master key does not match and the model invokes `list_secret_keys` or `discover_secrets`
- **THEN** the Tool returns the key names and metadata without error

#### Scenario: Specific unreadable definition reported

- **WHEN** the master key matches but a specific sensitive definition cannot be decrypted
- **THEN** the value-consuming Tool returns that definition's error, distinct from a key mismatch

#### Scenario: Classified errors carry no values

- **WHEN** a value-consuming Tool returns a classified vault error
- **THEN** the error contains no secret value
