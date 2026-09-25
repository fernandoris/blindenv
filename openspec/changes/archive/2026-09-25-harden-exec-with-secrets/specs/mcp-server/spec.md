## MODIFIED Requirements

### Requirement: Tool execute_with_secrets

The Tool SHALL run a local subprocess injecting the effective secrets of the resolved project and environment into its environment, capture a bounded amount of its output, apply the redaction engine, and return the redacted output along with the exit code. The effective secrets SHALL include the shared scopes when they are not shadowed by a more specific scope. The memory used to capture output MUST NOT grow with the child's total output volume: once a fixed maximum is retained, further bytes MUST be discarded rather than stored. The Tool MUST return within its execution timeout even when the child, or any descendant that inherited its stdout/stderr, keeps those streams open. On completion or timeout the Tool MUST terminate the entire child process tree.

#### Scenario: Project without execution permission

- **WHEN** the model invokes `execute_with_secrets` on a project with `allow_execute` disabled
- **THEN** the system refuses execution without running any subprocess

#### Scenario: Execution with injected secrets

- **WHEN** `allow_execute` is enabled for the project
- **THEN** the subprocess receives the effective secrets in its environment and its exit code is returned to the model

#### Scenario: Shared secret injected

- **WHEN** a key is defined only in a shared scope and the model runs a command
- **THEN** the subprocess receives that value in its environment

#### Scenario: Redacted output

- **WHEN** the subprocess prints a secret value in its output
- **THEN** the response returned to the model contains the redaction marker instead of the value

#### Scenario: Execution without a shell

- **WHEN** the model invokes the Tool with a separate command and arguments
- **THEN** the system runs the command directly without interpreting it through a shell

#### Scenario: Output larger than the retained maximum

- **WHEN** a command emits far more output than the retention maximum
- **THEN** the process memory used stays bounded, the response contains the retained prefix with a truncation indicator, and the exit code is reported

#### Scenario: Descendant keeps the stream open

- **WHEN** a command spawns a background descendant that inherits stdout/stderr and the direct child exits
- **THEN** the Tool terminates the process tree and returns within the execution timeout

#### Scenario: Command exceeds the timeout

- **WHEN** a command keeps running beyond the execution timeout
- **THEN** the Tool returns a timeout error within the deadline and the child process tree is terminated
