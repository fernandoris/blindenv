## Context

See `proposal.md` - Why. BlindEnv's MCP server runs as a stdio process on the
same desktop as the agent, so "open a browser" is a local OS call, not a network
transport. The existing secret-consuming tools already give us the two pieces we
reuse: in-process substitution (`proxy.go`, `substitute`) and per-project
capability gating with a default-off flag (`allow_execute` in `exec.go` +
`get_context`). This change adds a third sink that reuses both but has a
different residual-leak profile.

## Goals / Non-Goals

**Goals:**
- Resolve `{{SECRET_NAME}}` in a URL in-process and hand the resolved URL to the
  OS default browser launcher.
- Keep the resolved URL out of the model context, the tool result, the audit log
  and any error message.
- Gate the capability per project, default off, consistent with `allow_execute`.
- Stay dependency-free and cross-platform with `os/exec`.

**Non-Goals:**
- Remote/SSH/headless browser opening (agent on a different machine than the
  browser). Out of scope: there is no transport.
- Choosing a browser, a private/incognito window or an ephemeral profile. The
  default handler cannot do this reliably.
- Preventing the resolved URL from appearing in `ps` argv or browser history;
  both are accepted and documented (see Risks).
- Guaranteeing a tab opened: the launcher forks.

## Decisions

### D1: New tool `open_in_browser`, not an action on `proxy_http_request`

A new tool gets its own enable flag, its own audit semantics and its own
threat-model paragraph. Overloading `proxy_http_request` with a GUI side effect
would conflate a network verb with a local process launch and force its
substitution/redirect logic to grow a branch it does not need.

- **Alternatives considered**: (a) `proxy_http_request` with `action: open` -
  rejected, wrong abstraction; (b) a general `open_url` capability with a
  `target` argument for future sinks - rejected as speculative generality.

### D2: New per-project `allow_open`, default off, independent of `allow_execute`

The risk profile differs: `allow_execute` enables arbitrary code execution,
`allow_open` enables moving a secret into a process the agent may be able to
scrape back. Conflating them would silently grant the more dangerous new sink to
every project that already runs commands. Separate flag, separate decision.

- **Alternatives considered**: reuse `allow_execute` - rejected (above).
- **Migration**: additive column with default `false`; existing projects migrate
  disabled. Same migration mechanism already used for other schema additions.

### D3: Invoke the launcher directly, no shell

`exec.Command("open", url)` / `exec.Command("xdg-open", url)` /
`exec.Command("rundll32", "url.dll,FileProtocolHandler", url)`. No `sh -c`, so
shell metacharacters in the URL can never be interpreted. This mirrors the
no-shell guarantee that `execute_with_secrets` provides.

- **Alternatives considered**: `sh -c "open '$url'"` - rejected outright
  (injection); `osascript` on macOS - rejected, adds a scripting interpreter for
  no benefit.

### D4: Resolve only a URL string, reuse `substitute`

The handler resolves tags with the existing `substitute` helper over the URL and
passes the single resolved string to the launcher. It does not reuse the
proxy's header/body machinery, which is irrelevant here.

### D5: Result is "handed to the launcher", not "opened"

`open`/`xdg-open`/`rundll32` fork and return before a tab exists, so the only
honest report is that the URL was handed to the OS launcher. The result payload
contains no URL.

### D6: Linux headless guard

When `runtime.GOOS == "linux"` and both `DISPLAY` and `WAYLAND_DISPLAY` are
empty, refuse with an actionable error. `xdg-open` otherwise blocks or fails
opaquely on a headless box.

### D7: Audit stores the unsubstituted template

Audit `command` field = the URL as received, with `{{...}}` tags intact, plus
key names and source scopes. This is stronger than `proxy`'s `safeURL`, which
only strips the query string; here there is no resolved URL anywhere in the
audit path.

## Risks / Trade-offs

- **Resolved URL briefly visible in `ps` argv** → unavoidable: the OS launchers
  only accept the URL as a command-line argument (no stdin/env channel). Local
  only, and the launcher exits in milliseconds. Documented in the README threat
  model. Strictly better than the status quo, where the URL would also be
  recorded in the audit `command` field.
- **Resolved URL persists in browser history** → accepted: the default handler
  cannot force a private window. Documented in the README. Users who care must
  not put long-lived secrets in a URL.
- **Read-back by agent browser automation** → accepted as a documented caveat,
  same class as "malicious command exfiltration": if the agent has a browser
  tool (Playwright/Chrome DevTools MCP), it can scrape the tab. Added verbatim
  to the README threat model so no one mistakes this for a sandbox.
- **`xdg-open` absent on minimal Linux images** → the refusal/error path covers
  it with a clear message; no new dependency is introduced.
- **Audit `command` length** → bounded naturally by URL length; no buffering or
  redaction pass, so no memory-growth concern.

## Migration Plan

1. Add the additive `allow_open` column (default `false`) with the existing
   migration mechanism; opening a pre-change vault sets it disabled everywhere.
2. Deploy the new binary. MCP clients restart their `blindenv mcp` process on
   demand and pick it up.
3. Rollback: the feature is inert while `allow_open` is disabled; reverting the
   binary leaves the extra column unused. Recommend exporting a backup before
   upgrade, as with any migration.
