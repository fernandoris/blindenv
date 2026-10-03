## ADDED Requirements

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

## MODIFIED Requirements

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
