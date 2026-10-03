## ADDED Requirements

### Requirement: Migration notices on standard error

When an upgrade migrates the vault, the binary SHALL report the migration and the location of the pre-migration snapshot on standard error. Such notices MUST NOT be written to standard output, so the MCP JSON-RPC channel on standard output is never corrupted. The notice SHALL be emitted only when a migration actually changed the vault.

#### Scenario: Upgrade notice goes to standard error

- **WHEN** an older vault is opened by a newer binary and is migrated
- **THEN** a notice naming the old and new versions and the snapshot location is written to standard error
- **AND** standard output receives no migration notice

#### Scenario: No notice when already current

- **WHEN** a vault already at the supported version is opened
- **THEN** no migration notice is written

#### Scenario: Standard output stays clean for MCP

- **WHEN** the `mcp` subcommand starts after a migration
- **THEN** standard output carries only JSON-RPC and the migration notice appears on standard error
