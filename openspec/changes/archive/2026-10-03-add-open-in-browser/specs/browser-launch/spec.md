## Purpose

Lets an agent hand a fully resolved URL to the local default browser without the secret value ever entering the model context, the audit log or the command line.

## ADDED Requirements

### Requirement: Resolve-in-process browser open

The capability SHALL resolve `{{SECRET_NAME}}` tags inside a URL within BlindEnv and hand the resolved URL to the operating system's default browser launcher. The resolved URL MUST be passed to the launcher directly as an argument, without a shell, so the URL is never interpreted by a shell. A `{{SECRET_NAME}}` tag that does not name an effective key SHALL be left unchanged. A `{{SECRET_NAME}}` tag that names an effective key SHALL be substituted with that key's effective value, whether the key is sensitive or non-sensitive.

#### Scenario: Sensitive tag resolved for the browser

- **WHEN** the agent opens `https://idp.example/authorize?token={{SSO_TOKEN}}` and `SSO_TOKEN` is an effective sensitive key
- **THEN** the browser launcher receives the URL with the real token substituted and the agent receives no part of the value

#### Scenario: Non-sensitive tag resolved for the browser

- **WHEN** the agent opens `{{BASE_URL}}/dashboard` and `BASE_URL` is an effective non-sensitive key
- **THEN** the browser launcher receives the URL with the configuration value substituted

#### Scenario: Non-matching tag passes through unchanged

- **WHEN** the URL contains a `{{...}}` tag whose name is not an effective key
- **THEN** the tag is left unchanged in the URL handed to the launcher

#### Scenario: No shell interprets the URL

- **WHEN** the resolved URL contains shell metacharacters
- **THEN** the launcher receives it as a single argument and no shell processes it

### Requirement: Default-browser sink

The capability SHALL open the resolved URL in the operating system's default browser by invoking the platform launcher: `open` on macOS, `xdg-open` on Linux and `rundll32 url.dll,FileProtocolHandler` on Windows. The capability MUST NOT attempt to select a specific browser, a private/incognito window or an ephemeral profile.

#### Scenario: macOS launcher

- **WHEN** the agent opens a URL on macOS
- **THEN** the capability invokes `open` with the resolved URL

#### Scenario: Linux launcher

- **WHEN** the agent opens a URL on Linux with a display available
- **THEN** the capability invokes `xdg-open` with the resolved URL

#### Scenario: Windows launcher

- **WHEN** the agent opens a URL on Windows
- **THEN** the capability invokes `rundll32` with `url.dll,FileProtocolHandler` and the resolved URL

#### Scenario: A tab is not guaranteed

- **WHEN** the launcher returns successfully
- **THEN** the capability reports that the URL was handed to the OS launcher and does not claim the tab opened

### Requirement: Browser open requires an explicit environment

The capability SHALL require a non-empty resolved environment, like the other secret-consuming tools, and SHALL refuse the call with an error explaining that an environment is required when neither the call nor the pinned configuration provides one.

#### Scenario: Missing environment refused

- **WHEN** the agent opens a URL and no environment is provided by the call or the configuration
- **THEN** the capability refuses without launching the browser

### Requirement: Browser open requires the allow_open capability

The capability SHALL refuse to open a URL for a project whose `allow_open` capability is disabled, and the refusal SHALL not launch the browser or substitute any secret. The refusal SHALL explain how to enable the capability.

#### Scenario: Disabled capability refused

- **WHEN** the agent opens a URL for a project whose `allow_open` is disabled
- **THEN** the capability refuses with an error explaining how to enable it and opens nothing

#### Scenario: Enabled capability proceeds

- **WHEN** `allow_open` is enabled for the project
- **THEN** the capability resolves the URL and hands it to the launcher

### Requirement: Headless Linux refused

On Linux, when no display is available (`DISPLAY` and `WAYLAND_DISPLAY` are both empty), the capability SHALL refuse with an actionable error explaining that no graphical session was detected, instead of blocking or failing opaquely.

#### Scenario: No display detected

- **WHEN** the agent opens a URL on Linux and both `DISPLAY` and `WAYLAND_DISPLAY` are empty
- **THEN** the capability refuses with an error explaining that no display was detected and launches nothing

### Requirement: The resolved URL never reaches the agent or the audit log

The capability SHALL audit only the unsubstituted URL template, the project and environment, the tool name and the names of the keys used. The resolved URL MUST NOT appear in the audit entry, the tool result, or any error message returned to the agent; the result SHALL report only that the URL was handed to the launcher.

#### Scenario: Audit records the template

- **WHEN** the agent opens a URL containing `{{SSO_TOKEN}}`
- **THEN** the audit entry stores the URL with `{{SSO_TOKEN}}` intact and never the resolved value

#### Scenario: Result contains no URL

- **WHEN** the capability returns to the agent
- **THEN** the result reports that the URL was handed to the launcher and contains no part of the resolved URL or any secret value

#### Scenario: Error contains no resolved URL

- **WHEN** the launcher fails after substitution
- **THEN** the error returned to the agent contains no part of the resolved URL or any secret value
