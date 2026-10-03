## Context

See `proposal.md` for motivation.

- `proxy_http_request` substitutes `{{SECRET_NAME}}` with the value **in Go**, never through a shell (`pkg/mcp/proxy.go:129`).
- `execute_with_secrets` performs no substitution: it injects secrets as environment variables (`ChildEnv`, `pkg/mcp/exec.go:153`) and passes the command string to the shell (`shellCommand`, `pkg/mcp/exec.go:139`). A `{{...}}` tag reaches the shell verbatim.
- PowerShell tokenizes `{` / `}` as script-block delimiters and `@{` as a hashtable, so a tag inside a command is parsed as code. The agent is nudged toward PowerShell on Windows (`shellHint`, `pkg/mcp/server.go`).
- `get_context` already reports `shell_hint` and `allow_execute`, so it is the context-aware counterpart to the context-agnostic `discover_secrets`.

## Goals / Non-Goals

**Goals:**
- One portable tag (`{{SECRET_NAME}}`) for the whole product, working identically in `proxy` and `execute`.
- The secret value never enters the command line (`argv`); the shell reads it from the injected environment.
- No silent misbehavior: when translation cannot be guaranteed, fail with an actionable error.
- Keep the informational fallback available so an agent can write native syntax directly.

**Non-Goals:**
- Changing `proxy_http_request` (it stays value substitution in Go).
- `blindenv run` (no `shell` parameter; CLI users already write native shell syntax). Follow-up.
- Changing environment-variable naming or the storage/resolution model.
- Substituting values into commands.

## Decisions

### 1. One canonical tag, two mechanisms

`{{SECRET_NAME}}` keeps its meaning — "the secret value appears here" — and each surface realizes it safely: `proxy` replaces the value in the request (in-process); `execute` replaces the tag with the shell's native environment reference so the shell expands the value at runtime. The agent never needs per-shell syntax knowledge.

Alternative: a distinct new tag for commands. Rejected: two syntaxes for the same idea is less transparent.

### 2. Translate to a reference, not to the value

Replacing `{{NAME}}` with `"$env:NAME"` / `%NAME%` / `"${NAME}"` keeps the value out of `argv` (not visible via `ps`/process listing, not in the audit command) and avoids quoting/injection. Values keep flowing through the environment, consistent with the redaction model.

Alternative: substitute the value and quote it for the shell. Rejected: leaks the value into `argv` and the audit command, and needs per-shell escaping.

### 3. Shell classification and reference map

The shell is the `shell` argument (or none). Recognized shells map to a brace-form reference that tolerates arbitrary key names:

```
powershell / pwsh / *.exe   ->  ${env:NAME}
cmd / cmd.exe               ->  %NAME%
sh / bash / zsh / dash / ksh / ash / fish -> ${NAME}
(empty / unknown)           ->  none
```

Brace forms keep keys with non-identifier characters addressable (`${env:MY-KEY}`), matching the raw env-var injection in `ChildEnv`.

### 4. Quote-aware, fail-loud translation

Translate only tags whose name matches an effective key; every other `{{...}}` passes through untouched. Track single/double quote state for the target shell:

- outside a single-quoted region -> rewrite to the reference;
- inside a single-quoted region (POSIX/PowerShell, where references do not expand) -> refuse with an actionable error;
- shell empty or unknown while a matching tag is present -> refuse.

This makes the failure mode loud instead of a literal `{{NAME}}` or an unexpanded `${...}`. `cmd` has no single-quote semantics, so its tags always translate.

### 5. `get_context` exposes the reference

`get_context` gains an optional `shell` argument and a `secret_reference` field (`shell`, `native`, `tag`), defaulting to the OS shell. This is the informational half (A): if an agent prefers to write native syntax, or translation is refused, it has the exact reference. It also keeps discovery (`discover_secrets`) context-agnostic.

### 6. Scope: MCP only

`blindenv run` is excluded: it runs the command directly without a `shell` parameter, so shell detection would need inference or a new flag. Left as a follow-up to keep this change small and testable.

## Risks / Trade-offs

- [Shell quote parsing is error-prone] -> Restrict the rule to single-quote detection for the recognized shells, cover it with table-driven tests per shell, and prefer a loud error over a guess.
- [A legitimate literal `{{NAME}}` that matches a key would be rewritten] -> Only matching keys are touched; document an escape hatch (e.g. `\{{NAME}}`) as a follow-up if needed.
- [Unknown shells cannot translate] -> `get_context` gives the native reference; the error names it.
- [Windows `cmd` delayed-expansion edge cases] -> `%NAME%` is expanded at parse time in the common cases; tests cover the plain and parenthesized forms.
- [Recommendation vs. reality] `shell_hint` may differ from the actual `shell` used -> translation keys off the `shell` argument, and `get_context` accepts `shell` so the hint can be corrected.
- [Translation cost] A naive `for key: ReplaceAll` is O(keys x command length) with one copy per key (as `proxy.substitute` does today) -> implement a single-pass scan that resolves tags by name, O(command length), and guard it with a benchmark plus `testing.AllocsPerRun` on the no-tag case so it cannot regress.

## Migration Plan

Behavior addition with one behavior change: a command that previously carried a literal `{{NAME}}` matching a key now translates (known shell) or refuses (unknown shell). Commands without tags and non-matching tags are unaffected. Document in README and server instructions; rollback is reverting the translation step.

## Open Questions

- Whether to support an escape sequence for a literal matching tag in the first release; can ship without it.
