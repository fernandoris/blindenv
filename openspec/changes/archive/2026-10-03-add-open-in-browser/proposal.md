## Why

An agent sometimes needs to hand a *resolved* URL to the local browser — a signed
SSO link, a pre-authenticated dashboard, a deep link that embeds a token — but
today there is no way to do that without the value crossing the model context.
`execute_with_secrets` can open a URL, yet the resolved URL lands in the command
line, the audit `command` field and any `ps` snapshot. BlindEnv needs a local
browser-open sink that resolves `{{SECRET_NAME}}` in-process and never places the
value where the agent or the audit log can read it back.

## What Changes

- Add a new MCP tool `open_in_browser` that resolves `{{SECRET_NAME}}` tags in a
  URL inside BlindEnv and hands the resolved URL to the OS default browser
  launcher (`open`, `xdg-open`, `rundll32 url.dll,FileProtocolHandler`).
- Invoke the launcher **directly, without a shell**, so the URL is never
  interpreted (no `sh -c`).
- Gate the tool behind a new per-project capability `allow_open`, **disabled by
  default**, mirroring `allow_execute`.
- Report `allow_open` from `get_context`, like `allow_execute`.
- Add the `allow_open` toggle to the project view in the local dashboard.
- Audit only the **unsubstituted URL template** and the key names — never the
  resolved URL.
- On Linux, detect a missing display (`DISPLAY`/`WAYLAND_DISPLAY` empty) and
  refuse with an actionable error instead of blocking or failing opaquely.
- Document the accepted residual leaks in the README threat model: the resolved
  URL is briefly visible in `ps` argv, persists in browser history (the default
  handler cannot open a private window), and can be read back by any browser
  automation the agent controls.

## Capabilities

### New Capabilities

- `browser-launch`: resolving `{{SECRET_NAME}}` tags into a URL in-process and
  handing the resolved URL to the OS default browser without exposing the value
  to the agent, the audit log or the command line.

### Modified Capabilities

- `mcp-server`: adds the `open_in_browser` tool, its `allow_open` gate, the
  `allow_open` field in `get_context`, and template-only auditing.
- `vault`: adds the per-project `allow_open` capability (schema + model),
  defaulting to disabled.
- `web-ui`: adds the `allow_open` toggle to the project view, mirroring
  `allow_execute`.

## Impact

- `pkg/mcp`: new `open.go` (gate, in-process substitution, per-OS launcher,
  audit), tool registration and handler in `tools.go`, `allow_open` in
  `get_context` in `tools.go`.
- `pkg/db`: new `allow_open` column on projects (migration), model and
  read/write paths in `models.go`/`store.go`/`schema.go`.
- `pkg/web`: project flag update endpoint and UI control in `server.go` and
  `web/app.js`.
- `README.md`: tool list, feature list, threat-model bullet and CLI/tool docs.
- No new dependencies: the launcher uses `os/exec` with the standard OS opener.
