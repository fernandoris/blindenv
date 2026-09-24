## Purpose

Defines the command-line surface of the `blindenv` binary, bounded to which subcommands exist and what may be written to standard output.

## ADDED Requirements

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

The `run` subcommand SHALL execute a child command injecting the secrets of the indicated project and environment, without printing the values, and SHALL propagate the child's exit code.

#### Scenario: Running in CI or scripts

- **WHEN** the user runs `blindenv run <project>/<environment> -- <command>`
- **THEN** the command runs with the secrets in its environment and the value is not printed

#### Scenario: Exit code propagation

- **WHEN** the child command exits with a non-zero code
- **THEN** `blindenv` returns that same exit code

### Requirement: version command

The `version` subcommand SHALL print the BlindEnv version in a readable form.

#### Scenario: Version query

- **WHEN** the user runs `blindenv version`
- **THEN** the system prints the version and exits with code zero

### Requirement: Vault location

The binary SHALL resolve the vault path under the user's configuration directory in a cross-platform way, and SHALL allow it to be explicitly overridden.

#### Scenario: Default path

- **WHEN** the user does not provide a vault path
- **THEN** the system uses the operating system's user configuration directory

#### Scenario: Explicit path

- **WHEN** the user provides an explicit vault path
- **THEN** the system operates on that path
