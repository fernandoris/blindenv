# mcp-server Specification

## Purpose

Exposes the vault to AI agents through the Model Context Protocol, letting them use and reference dev/pre-prod secrets without the values entering the model context window.

## Requirements

### Requirement: stdio transport with reserved stdout

The MCP server SHALL communicate over `stdio` using JSON-RPC. The process MUST NOT write anything to stdout outside the protocol stream; logs MUST go to stderr or the audit log.

#### Scenario: Startup without corrupting the protocol

- **WHEN** the MCP server starts and emits log messages
- **THEN** logs are written to stderr and stdout contains only MCP protocol messages

### Requirement: Tool list_secret_keys

The Tool SHALL return only the NAMES of the effective keys for the resolved project and environment, drawn from all applicable scopes (project + environment, project-global, environment-global and global) without duplicates, never their values.

#### Scenario: Names without values

- **WHEN** the model invokes `list_secret_keys`
- **THEN** the response contains only key names and no value

#### Scenario: Union of global and environment

- **WHEN** the vault defines a global key, an environment-global key for the queried environment, a project-global key and a project + environment key
- **THEN** the response includes all four names, each once

#### Scenario: Shared keys appear in every matching project

- **WHEN** a global key is defined and the model queries any project and environment
- **THEN** the response includes that key name

### Requirement: Tool discover_secrets

The Tool SHALL return the NAMES of the secret keys available to the resolved project, grouped by scope, without returning any secret VALUE. It SHALL include the shared global scope, the environment-global scope for every environment name that carries at least one such key, the project-global scope of the resolved project, and the project + environment scope for every environment of the project that defines one. For every key it SHALL also report the type and, when present, the hint, and whether the definition is non-sensitive. The Tool MUST be independent of the resolved environment: it SHALL NOT hide environment-scoped keys when the resolved environment is empty or different, and it SHALL NOT fail because a shared environment name has no matching environment in the project.

#### Scenario: Names, scope and environment without values

- **WHEN** the model invokes `discover_secrets`
- **THEN** the response contains key names with their scope and, for environment-scoped keys, the environment name, and no secret value

#### Scenario: Metadata reported per key

- **WHEN** the model invokes `discover_secrets`
- **THEN** every key reports its type, its hint when present, and whether it is non-sensitive

#### Scenario: Environment-scoped keys visible regardless of resolved environment

- **WHEN** an environment-global key exists for environment `DES` and the resolved environment is empty or a different name
- **THEN** the response includes that key under scope `environment` with environment `DES`

#### Scenario: Shared environment without a project environment

- **WHEN** an environment-global key exists for environment `DES` and the resolved project has no environment named `DES`
- **THEN** the response lists that key instead of failing

#### Scenario: Grouped by scope

- **WHEN** the vault defines a global key, an environment-global key, a project-global key and a project + environment key
- **THEN** the response places each key under its corresponding scope group and environment

#### Scenario: Discovery never returns values

- **WHEN** the model invokes `discover_secrets`
- **THEN** the response contains only names, metadata, scopes and environment names, and no secret value, including for non-sensitive keys

### Requirement: Tool get_context

The Tool SHALL return the execution context: operating system, architecture, shell hint, the resolved shell's secret reference, active project and environment, whether execution with secrets is enabled for the resolved project, whether opening a URL in the browser is enabled for the resolved project, and the effective keys with the source scope that supplied each value. For each key it SHALL report its type, its hint when present, and whether it is non-sensitive, alongside the source-scope information. The secret reference SHALL report the canonical tag `{{SECRET_NAME}}` and the native environment reference for that shell (`${env:NAME}` for PowerShell/pwsh, `%NAME%` for cmd, `${NAME}` for POSIX shells). The Tool SHALL accept an optional `shell` argument and SHALL default to the OS shell when it is not provided. For each key it SHALL report one of the four scopes (`global`, `environment`, `project`, `project_environment`), the environment name when the source scope is environment-specific, and the broader scopes the definition shadows. The Tool SHALL return the value of a key only when that key's effective definition is non-sensitive; it MUST NOT return the value of a sensitive key.

#### Scenario: Context without values

- **WHEN** the model invokes `get_context` and every effective key is sensitive
- **THEN** the response includes the operating system, the project and environment, and the effective keys, but no value

#### Scenario: Metadata reported per key

- **WHEN** the model invokes `get_context`
- **THEN** every effective key reports its type, its hint when present, and whether it is non-sensitive

#### Scenario: Non-sensitive value returned

- **WHEN** an effective key's winning definition is non-sensitive
- **THEN** the response includes that key's value

#### Scenario: Sensitive value withheld

- **WHEN** an effective key's winning definition is sensitive
- **THEN** the response does not include that key's value

#### Scenario: Execution capability reported

- **WHEN** the model invokes `get_context` for a project whose `allow_execute` is disabled or enabled
- **THEN** the response reports the matching execution capability for that project, before the model attempts `execute_with_secrets`

#### Scenario: Browser capability reported

- **WHEN** the model invokes `get_context` for a project whose `allow_open` is disabled or enabled
- **THEN** the response reports the matching browser-open capability for that project, before the model attempts `open_in_browser`

#### Scenario: Shared key source reported

- **WHEN** a key's effective value resolves from the global scope or from the environment-global scope for the queried environment
- **THEN** the response reports that shared scope as the source, and includes the environment name when the source is environment-global

#### Scenario: Project-scoped key source reported

- **WHEN** a key is defined only in the project-global scope or in the project + environment scope
- **THEN** the response reports that project scope as the source, so the model can tell a project-local key from a shared one

#### Scenario: Shadowed scopes reported

- **WHEN** a key is defined in more than one applicable scope
- **THEN** the response reports the winning scope and lists the broader scopes the definition shadows

#### Scenario: Secret reference reported

- **WHEN** the model invokes `get_context` with a shell, or without one
- **THEN** the response reports the canonical tag `{{SECRET_NAME}}` and the native environment reference for that shell, defaulting to the OS shell

### Requirement: Tool proxy_http_request

The Tool SHALL perform HTTP requests substituting `{{SECRET_NAME}}` tags in URL, headers and body inside BlindEnv, using the secrets of the resolved project and environment.

#### Scenario: Substitution in a header

- **WHEN** a header contains `{{API_KEY}}`
- **THEN** the outgoing request sends the real value and the model never receives it

#### Scenario: JSON body with quotes

- **WHEN** a secret value contains special characters and is substituted into a JSON body
- **THEN** the resulting body remains valid JSON

#### Scenario: Cross-host redirect

- **WHEN** a response redirects to a different host
- **THEN** the system does not forward secret-bearing headers to the new host

### Requirement: Tool execute_with_secrets

The Tool SHALL run a local subprocess injecting the effective secrets of the resolved project and environment into its environment, capture a bounded amount of its output, apply the redaction engine, and return the redacted output along with the exit code. The effective secrets SHALL include the shared scopes when they are not shadowed by a more specific scope. Before running, the Tool SHALL translate every `{{SECRET_NAME}}` tag that names an effective key into the target shell's native environment reference, so the value is read from the injected environment and never placed on the command line; a tag whose name is not an effective key SHALL pass through unchanged. When the target shell is unknown or empty, or a matching tag appears inside a single-quoted region where the shell would not expand the reference, the Tool SHALL refuse the call with an error explaining how to proceed, rather than passing the tag through or embedding the value. The memory used to capture output MUST NOT grow with the child's total output volume: once a fixed maximum is retained, further bytes MUST be discarded rather than stored. The Tool MUST return within its execution timeout even when the child, or any descendant that inherited its stdout/stderr, keeps those streams open. On completion or timeout the Tool MUST terminate the entire child process tree.

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

#### Scenario: Tag translated to the shell reference

- **WHEN** a command for a known shell contains `{{API_KEY}}` and `API_KEY` is an effective key
- **THEN** the subprocess receives the equivalent native reference (`${env:API_KEY}`, `%API_KEY%` or `${API_KEY}`) and the value is never placed on the command line

#### Scenario: Non-matching tag passes through

- **WHEN** a command contains a `{{...}}` tag whose name is not an effective key
- **THEN** the tag is left unchanged for the shell to interpret

#### Scenario: Tag inside single quotes refused

- **WHEN** a matching `{{API_KEY}}` appears inside a single-quoted region for a shell that does not expand references there
- **THEN** the Tool refuses with an error explaining to use the native reference instead, without running the command

#### Scenario: Unknown shell with a matching tag refused

- **WHEN** a command contains a matching tag and the resolved shell is unknown or empty
- **THEN** the Tool refuses with an error explaining that a shell is required to translate the tag, without running the command

### Requirement: Tool open_in_browser

The Tool SHALL resolve `{{SECRET_NAME}}` tags in a URL inside BlindEnv using the secrets of the resolved project and environment, then hand the resolved URL to the operating system's default browser launcher. It SHALL accept an optional `url` (required), `project` and `environment`. It SHALL require a non-empty environment like the other secret-consuming Tools. It SHALL invoke the launcher directly, without a shell. A `{{SECRET_NAME}}` tag that does not name an effective key SHALL pass through unchanged. The Tool SHALL report only that the URL was handed to the launcher and MUST NOT return any part of the resolved URL or any secret value.

#### Scenario: Sensitive tag resolved for the browser

- **WHEN** the model opens a URL containing `{{SSO_TOKEN}}` and `SSO_TOKEN` is an effective sensitive key
- **THEN** the launcher receives the URL with the value substituted and the model never receives it

#### Scenario: Non-matching tag passes through

- **WHEN** a URL contains a `{{...}}` tag whose name is not an effective key
- **THEN** the tag is left unchanged in the URL handed to the launcher

#### Scenario: Empty environment refused

- **WHEN** the model invokes `open_in_browser` and neither the call nor the pinned configuration provides an environment
- **THEN** the system refuses with an error explaining that an environment is required, without launching the browser

#### Scenario: Result contains no URL

- **WHEN** the Tool returns to the model
- **THEN** the response reports that the URL was handed to the launcher and contains no part of the resolved URL or any secret value

### Requirement: open_in_browser requires the allow_open capability

The Tool SHALL refuse to open a URL for a project whose `allow_open` capability is disabled, without launching the browser or substituting any secret, and SHALL explain how to enable the capability.

#### Scenario: Disabled capability refused

- **WHEN** the model invokes `open_in_browser` on a project with `allow_open` disabled
- **THEN** the system refuses with an error explaining how to enable it without launching the browser

### Requirement: No display refused on Linux

On Linux, when neither `DISPLAY` nor `WAYLAND_DISPLAY` is set, the Tool SHALL refuse with an actionable error explaining that no graphical session was detected, instead of blocking or failing opaquely.

#### Scenario: Headless Linux refused

- **WHEN** the model invokes `open_in_browser` on Linux with no display available
- **THEN** the system refuses with an error explaining that no display was detected and launches nothing

### Requirement: Audit of browser opens records only the template

The server SHALL log an `open_in_browser` invocation with timestamp, project, environment, Tool name, the names of the keys used with their source scopes, and the **unsubstituted URL template**. The audit entry MUST NOT contain the resolved URL or any secret value.

#### Scenario: Audit stores the template

- **WHEN** the model opens a URL containing `{{SSO_TOKEN}}`
- **THEN** the audit entry stores the URL with `{{SSO_TOKEN}}` intact and never the resolved value

### Requirement: Discovery phase in server guidance

The server SHALL advertise a discovery-first workflow in its MCP instructions: the model discovers available keys with `discover_secrets`, then selects a project and environment and reads the shell's secret reference from `get_context`, then uses secrets through `proxy_http_request` or `execute_with_secrets`.

#### Scenario: Guidance advertises discovery

- **WHEN** an MCP client reads the server instructions
- **THEN** the instructions describe `discover_secrets` and the discover-then-use order before the secret-consuming tools

#### Scenario: Guidance describes the reference step

- **WHEN** an MCP client reads the server instructions
- **THEN** the instructions describe reading the shell's secret reference from `get_context` between discovery and use

### Requirement: Context pinned in configuration with override

The server SHALL take the project and environment from the MCP client configuration when the Tool does not receive them, and SHALL allow overriding them per call when provided. The server MUST require a non-empty resolved environment for the secret-consuming tools (`proxy_http_request`, `execute_with_secrets` and `open_in_browser`); a call that resolves to an empty environment SHALL be refused with an error explaining that an environment is required. The discovery and listing tools (`discover_secrets`, `list_secret_keys`, `get_context`) SHALL NOT require an environment.

#### Scenario: Using the pinned context

- **WHEN** the model invokes a Tool without specifying project or environment
- **THEN** the system uses the project and environment pinned in the configuration

#### Scenario: Per-call override

- **WHEN** the model invokes a Tool specifying an environment different from the pinned one
- **THEN** the system uses the environment given in the call

#### Scenario: Empty environment refused for secret-consuming tools

- **WHEN** the model invokes `proxy_http_request`, `execute_with_secrets` or `open_in_browser` and neither the call nor the pinned configuration provides an environment
- **THEN** the system refuses the call with an error explaining that an environment is required, without performing the request, running any command or launching the browser

#### Scenario: Listing tools allow an empty environment

- **WHEN** the model invokes `discover_secrets`, `list_secret_keys` or `get_context` and no environment is provided
- **THEN** the system returns the available names without requiring an environment

### Requirement: Audit without values

The server SHALL log every Tool invocation with timestamp, project, environment, Tool name, NAMES of the keys used and the scope each key resolved from, command when applicable, exit code and redaction count. The log MUST NOT contain secret values.

#### Scenario: Execution logged

- **WHEN** the model uses a Tool that consumes secrets
- **THEN** the system appends an audit entry with the key names, their source scopes and without their values

#### Scenario: Shared source recorded

- **WHEN** a key used by a Tool resolved from a shared scope
- **THEN** the audit entry records that scope as the source of the key

### Requirement: Errors without values

Error messages returned to the model MUST NOT contain secret values.

#### Scenario: Command failure

- **WHEN** a command fails and its error output contained a secret value
- **THEN** the error returned to the model contains the redaction marker instead of the value

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

### Requirement: Non-sensitive configuration values in secret-consuming tools

The secret-consuming Tools SHALL inject a non-sensitive value into the child environment or substitute it in an HTTP request exactly as a sensitive value, but MUST NOT redact its literal occurrences from returned output. A `{{SECRET_NAME}}` tag that names a non-sensitive key SHALL still be substituted (proxy) or translated to the shell's native reference (execute).

#### Scenario: Non-sensitive value injected

- **WHEN** a non-sensitive key is effective and the model runs a command referencing it
- **THEN** the child process receives the value in its environment

#### Scenario: Non-sensitive value not redacted

- **WHEN** command output prints a non-sensitive value
- **THEN** the response returned to the model contains it unchanged

#### Scenario: Proxy substitutes a non-sensitive tag

- **WHEN** an HTTP header contains `{{ENDPOINT}}` and `ENDPOINT` is a non-sensitive key
- **THEN** the outgoing request carries the value and no error is raised

### Requirement: Substitution and unmatched-tag reporting

The secret-consuming Tools SHALL report, in their successful result payload, a request-side substitution count and the names of any `{{...}}` tags that did not name an effective key. The substitution count SHALL be the number of `{{SECRET_NAME}}` occurrences replaced (or, for `execute_with_secrets`, rewritten to the target shell's native environment reference) across the request the Tool sends: URL, header values and body for `proxy_http_request`; command and arguments for `execute_with_secrets`; URL for `open_in_browser`. A repeated tag SHALL count once per occurrence. A tag naming a non-sensitive effective key SHALL count as a substitution, consistent with it still being substituted. `unmatched_tags` SHALL report each distinct unmatched tag name at most once, SHALL NOT contain a tag name outside the input the caller supplied, and SHALL NOT contain any secret value. The reported list SHALL be bounded so it cannot grow with the request size. `redactions` SHALL remain a response-side count and MUST NOT be conflated with the substitution count.

#### Scenario: Proxy reports substituted occurrences

- **WHEN** `proxy_http_request` replaces a `{{SECRET_NAME}}` tag that appears in a header and again in the body
- **THEN** the result reports a substitution count of two

#### Scenario: Execute reports translated references

- **WHEN** `execute_with_secrets` rewrites matching `{{SECRET_NAME}}` tags in a command to the shell's native environment reference
- **THEN** the result reports a substitution count equal to the number of matching tags rewritten

#### Scenario: Open reports URL substitutions

- **WHEN** `open_in_browser` resolves a URL containing one `{{SECRET_NAME}}` tag that names an effective key
- **THEN** the result reports a substitution count of one

#### Scenario: No tag used

- **WHEN** a request contains no `{{...}}` tag
- **THEN** the result reports a substitution count of zero and an empty unmatched-tag list

#### Scenario: Unmatched tag reported by name

- **WHEN** a request contains `{{MISSING_KEY}}` and no effective key is named `MISSING_KEY`
- **THEN** the result reports zero substitutions and lists `MISSING_KEY` among the unmatched tags

#### Scenario: Repeated unmatched tag listed once

- **WHEN** the same unmatched tag name appears several times in one request
- **THEN** the result lists that name exactly once

#### Scenario: Substitution without response redaction

- **WHEN** a tag naming a sensitive effective key is substituted into the request and the response does not contain the value
- **THEN** the result reports the substitution count greater than zero and a redaction count of zero

#### Scenario: Unmatched tag list is bounded

- **WHEN** a request contains more distinct unmatched tag names than the reporting maximum
- **THEN** the result reports at most the fixed maximum distinct names without error

#### Scenario: Non-sensitive tag counts as a substitution

- **WHEN** a request substitutes `{{ENDPOINT}}` and `ENDPOINT` is a non-sensitive effective key
- **THEN** the result reports it as a substitution

### Requirement: Audit records the substitution count

The audit entry for a secret-consuming Tool invocation SHALL record the request-side substitution count alongside the existing redaction count. The audit entry MUST NOT contain secret values. Unmatched tag names SHALL NOT be persisted in the audit entry.

#### Scenario: Substitution count persisted

- **WHEN** `proxy_http_request` or `execute_with_secrets` substitutes one or more tags
- **THEN** the audit entry records the matching substitution count

#### Scenario: Refused invocation records zero

- **WHEN** a secret-consuming Tool refuses a call before substituting any tag
- **THEN** the audit entry records a substitution count of zero
