## MODIFIED Requirements

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
