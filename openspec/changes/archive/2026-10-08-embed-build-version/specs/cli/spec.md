## MODIFIED Requirements

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
