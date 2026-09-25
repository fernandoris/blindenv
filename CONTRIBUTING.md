# Contributing to BlindEnv

Thanks for your interest! BlindEnv is a small Go project with a strong bias
towards a clear, honest security model. Please read the threat model in the
[README](README.md) before proposing security-related changes.

## Getting started

```sh
git clone https://github.com/fernandoris/blindenv
cd blindenv
go build ./...
go test ./...
```

Requires Go 1.24+ (no CGO).

## Before opening a pull request

- Run `gofmt -w .`, `go vet ./...` and `go test ./...`.
- Keep changes focused; one concern per pull request.
- Add or update tests for behavior changes. Security-sensitive code (crypto,
  redaction, the web API boundary) must have tests.
- If you change the dashboard styles, rebuild the committed stylesheet:

  ```sh
  npx tailwindcss@3 -c tailwind.config.js -i web/src.css -o web/dist/app.css --minify
  ```

## Project layout

| Path | Purpose |
| --- | --- |
| `cmd/blindenv` | CLI entrypoint (`mcp`, `ui`, `run`, `backup`, `version`). |
| `pkg/crypto` | Key providers (keyring, passphrase) and AES-256-GCM. |
| `pkg/db` | Embedded SQLite repository and the four-tier secret scope model (global, environment-global, project-global, project + environment). |
| `pkg/mcp` | MCP server, tools, execution, HTTP proxy and redaction. |
| `pkg/backup` | Passphrase-encrypted vault export/import. |
| `pkg/web` + `web/` | Local dashboard and its embedded assets. |

## Design decisions

Significant design decisions live in `openspec/changes/`. The change
`add-blindenv-core` documents the rationale for the storage model, key
management, redaction and MCP tool surface; `add-shared-scopes` documents the
four-tier scope model and its precedence. Read them before making architectural
changes. Resolution order is project + environment > project-global >
environment-global > global; keep it in one place (`pkg/db/store.go`).

## Reporting security issues

Please do not open a public issue for a vulnerability. Contact the maintainer
privately instead.
