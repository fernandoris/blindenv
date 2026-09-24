## Why

Los agentes de IA (Claude Code, Cursor, opencode) necesitan secretos de dev/pre-prod para ser útiles, pero hoy la única forma de dártelos es pegarlos en un prompt, en un `.env` o en una variable de entorno. Eso filtra las credenciales al contexto del modelo, a los historiales y a los logs. No existe un vault local, cifrado y nativo de MCP que permita a la IA *usar* un secreto sin *verlo*.

## What Changes

- Nuevo binario Go `blindenv` (distribución en un único ejecutable, Windows como objetivo de primera clase) con comandos `mcp`, `ui`, `run` y `version`.
- **Vault local cifrado**: base SQLite embebida con cifrado AES-256-GCM por valor, clave maestra en el llavero del sistema operativo con fallback por passphrase (Argon2id).
- **Modelo de secretos**: proyecto (slug único) con secretos **globales** + secretos por **entorno**, donde el valor de entorno pisa al global (un solo nivel de fallback).
- **Servidor MCP por `stdio`** con Tools `list_secret_keys`, `get_context`, `proxy_http_request` y `execute_with_secrets`; `execute` desactivado por defecto y habilitable por proyecto (`allow_execute`).
- **Redaction Engine**: intercepta stdout/stderr de los comandos y sustituye valores sensibles por `[BLINDENV_REDACTED:<KEY>]`, con normalización de encoding previa (crítico en Windows/PowerShell), orden por longitud y umbral mínimo.
- **UI web local** (`127.0.0.1`) con Tailwind embebido vía `go:embed`, protegida por token y validación de `Host`.
- **Backup portable** cifrado con passphrase, para no perder el vault si desaparece la entrada del llavero.
- `README.md` de proyecto de software libre: propósito, threat model honesto (la redacción es DLP, no sandbox), instalación, e integración lista para opencode / Claude Code / Cursor.
- **BREAKING**: no aplica (proyecto nuevo, sin usuarios previos).

## Capabilities

### New Capabilities
- `vault`: almacenamiento local cifrado de secretos, gestión de clave maestra (keyring + fallback passphrase), modelo proyecto/global/entorno, y backup/restore portable.
- `secret-redaction`: motor de redacción que evita reemplazar valores sensibles en la salida de comandos, con reglas de encoding, orden y longitud.
- `mcp-server`: servidor MCP por stdio, catálogo de Tools, contexto de proyecto/entorno fijado en configuración, política de `execute_with_secrets` y registro de auditoría sin valores.
- `web-ui`: dashboard local para gestionar proyectos, globales, entornos y secretos, revisar auditoría y exportar/importar el vault.
- `cli`: superficie de línea de comandos del binario (`mcp`, `ui`, `run`, `version`) y su contrato de salida (stdout reservado al protocolo MCP).

### Modified Capabilities
- Ninguna (proyecto greenfield; no hay specs existentes).

## Impact

- **Código nuevo**: estructura `cmd/blindenv` + `pkg/{crypto,db,mcp,web}` + `web/` (assets), todo en Go.
- **Dependencias**: `github.com/mark3labs/mcp-go`, `modernc.org/sqlite` (Go puro, sin CGO), `github.com/zalando/go-keyring`, `golang.org/x/crypto/argon2`. Build de UI requiere Node (Tailwind CLI) solo en tiempo de compilación.
- **Plataformas objetivo**: Windows (primera clase), macOS y Linux; cross-compile sin C toolchain.
- **Operación**: vault en `os.UserConfigDir()/blindenv/`; concurrencia multi-proceso sobre SQLite en modo WAL (varios clientes MCP + UI).
- **Documentación**: `README.md` en inglés (estándar de software libre) con sección de integración por cliente.
- **No-objetivos v1**: transporte MCP HTTP/SSE, aprobación interactiva por llamada, herencia multi-nivel de entornos, servidor remoto/multiusuario.
