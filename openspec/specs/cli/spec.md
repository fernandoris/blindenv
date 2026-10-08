# cli Specification

## Purpose

Defines the command-line surface of the `blindenv` binary, bounded to which subcommands exist and what may be written to standard output.

## Requirements

### Requirement: Binary subcommands

The binary SHALL provide the `mcp`, `ui`, `run` and `version` subcommands.

#### Scenario: Binary help

- **WHEN** the user runs `blindenv --help`
- **THEN** the system lists the available subcommands

#### Scenario: Unknown subcommand

- **WHEN** the user runs an unrecognized subcommand
- **THEN** the system shows an error and returns a non-zero exit code

### Requirement: No subcommand that prints values

The binary MUST NOT provide a subcommand that prints a secret value to stdout, to prevent an agent with shell access from bypassing redaction.

#### Scenario: Direct print attempt

- **WHEN** the available subcommands are inspected
- **THEN** none of them prints secret values to stdout

### Requirement: Running commands with secrets

The `run` subcommand SHALL execute a child command injecting the secrets of the indicated project and environment, without printing the values, and SHALL propagate the child's exit code. The `run` subcommand SHALL emit the child's output with values redacted without retaining the child's entire output in memory, and SHALL terminate the entire child process tree when the child exits, so that a background descendant cannot keep the command alive or block its return.

#### Scenario: Running in CI or scripts

- **WHEN** the user runs `blindenv run <project>/<environment> -- <command>`
- **THEN** the command runs with the secrets in its environment and the value is not printed

#### Scenario: Exit code propagation

- **WHEN** the child command exits with a non-zero code
- **THEN** `blindenv` returns that same exit code

#### Scenario: Verbose child output

- **WHEN** the child command emits far more output than any internal buffer
- **THEN** the memory used by `blindenv` stays bounded and the output continues to be emitted with secret values redacted

#### Scenario: Background descendant

- **WHEN** the child command spawns a background descendant that inherits stdout/stderr and then exits
- **THEN** `blindenv run` terminates the process tree and returns without hanging

### Requirement: version command

The `version` subcommand SHALL print the BlindEnv version together with a build identity in a readable form, and SHALL exit with code zero. The build identity SHALL include the source revision and whether the working tree was modified when the binary was built from a version-control checkout, or the module version when the binary was produced by `go install <module>@<version>`. The command MUST prefer a value injected at build time through `-ldflags` over any other source, and MUST NOT print the `none` / `unknown` placeholders when the Go build information carries an identity.

#### Scenario: Version query

- **WHEN** the user runs `blindenv version`
- **THEN** the system prints the version and its build identity and exits with code zero

#### Scenario: Build identity from build info

- **WHEN** the binary was built from a version-control checkout without `-ldflags` injection
- **THEN** the printed identity includes the source revision and whether the working tree was modified

#### Scenario: Build identity from a module install

- **WHEN** the binary was produced by `go install <module>@<version>`
- **THEN** the printed identity includes the installed module version

#### Scenario: Injected identity takes precedence

- **WHEN** the binary was built with a version, commit and date injected through `-ldflags`
- **THEN** the printed identity reports the injected values

#### Scenario: No placeholder when identity is available

- **WHEN** the binary carries a source revision or a module version
- **THEN** the command does not print `commit none` or `built unknown`

### Requirement: Vault location

The binary SHALL resolve the vault path under the user's configuration directory in a cross-platform way, and SHALL allow it to be explicitly overridden.

#### Scenario: Default path

- **WHEN** the user does not provide a vault path
- **THEN** the system uses the operating system's user configuration directory

#### Scenario: Explicit path

- **WHEN** the user provides an explicit vault path
- **THEN** the system operates on that path

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
