## Context

Proyecto greenfield (`BlindEnv`) sin código previo. Ver `proposal.md - Why` para la motivación. Este documento fija las decisiones técnicas acordadas y sus alternativas, para que la implementación no reabra debates ya zanjados.

Restricciones que moldean el diseño:
- Distribución en **un único binario** sin toolchain C → condiciona el driver SQLite.
- **Windows es objetivo de primera clase** (además de macOS/Linux) → condiciona encoding, shell y permisos.
- Varios clientes MCP (opencode, Claude Code, Cursor) lanzan cada uno su propio proceso → concurrencia multi-proceso sobre el mismo almacenamiento.
- El servidor MCP habla por `stdout` → la salida estándar es un canal reservado.

## Goals / Non-Goals

**Goals:**
- Que un agente de IA pueda *usar* secretos de dev/pre-prod sin que sus valores entren en el contexto del modelo ni en logs.
- Un único ejecutable multiplataforma, sin red por defecto y con el vault bajo control del usuario.
- Un threat model explícito y honesto, documentado en el `README.md`.

**Non-Goals:**
- Blindar secretos de producción o servir como gestor multiusuario/remoto.
- Garantizar que un comando no pueda exfiltrar un secreto por red (la redacción es DLP, no sandbox).
- Transporte MCP HTTP/SSE, aprobación interactiva por llamada y herencia multi-nivel (futuras versiones).

## Decisions

### D1. Cifrado por valor con AES-256-GCM, nombres en claro

Cada valor de secreto se cifra de forma independiente con AES-256-GCM y nonce aleatorio (prefijado al ciphertext). Los nombres de proyecto, entorno y clave quedan legibles.
- **Por qué**: permite actualizar un secreto sin re-cifrar el resto, es simple, y los nombres no son el secreto.
- **Alternativas**: (a) cifrar el archivo completo (SQLCipher) → acopla el driver y pierde pure-Go; (b) subclave por proyecto vía HKDF → aislamiento extra, pospuesto por complejidad innecesaria en dev/pre-prod.

### D2. KeyProvider enchufable: keyring-first + fallback passphrase

El módulo `crypto` solo conoce una clave de 32 bytes; su origen es un `KeyProvider` sustituible. Por defecto se usa el llavero del sistema operativo (`github.com/zalando/go-keyring`); si no está disponible, se deriva la clave de una passphrase con Argon2id (`golang.org/x/crypto/argon2`) y un salt persistido.
- **Por qué**: el llavero da cero fricción en el portátil; el fallback permite Docker/CI donde no hay Secret Service.
- **Alternativas**: (a) solo keyring → no funciona headless; (b) solo passphrase → fricción en cada uso.

### D3. Driver SQLite Go puro

`modernc.org/sqlite`.
- **Por qué**: cross-compila a Windows/Linux/macOS sin CGO ni mingw, requisito del binario único y de Windows como objetivo de primera clase.
- **Alternativas**: `mattn/go-sqlite3` (CGO, descartado), `zombiezen.com/go/sqlite` (válido, pero `modernc` es el más directo).

### D4. Modelo mínimo de secretos: proyecto + global + entorno (un solo fallback)

Un proyecto se identifica por slug único. Sus secretos pueden ser globales (aplican a todos los entornos) o de un entorno concreto. La resolución usa entorno-gana-global, con un único salto.
- **Por qué**: cubre el caso "misma key, distinto valor por entorno" sin la ambigüedad de la herencia en cadena.
- **Alternativas**: jerarquía/anidamiento (descartado por impredecible), identificadores UUID (descartado por ergonomía; el slug es más legible en configs), global cross-proyecto (fuera de alcance v1).

### D5. MCP por stdio en v1, con stdout reservado

El servidor usa `stdio` y JSON-RPC. Logs a stderr/archivo; nunca a stdout.
- **Por qué**: cero red, hereda la sesión del usuario y es compatible con todos los clientes.
- **Alternativas**: HTTP/SSE (requiere daemon, puerto y token; pospuesto a v2).

### D6. Integración MCP-first; sin subcomando que imprima valores

La IA usa los secretos exclusivamente vía Tools MCP. El CLI no expone `get`/`export` (evita que el agente eluda la redacción desde su shell). `run` queda para CI/scripts humanos.
- **Por qué**: cierra el footgun de `export API_KEY=$(blindenv get ...)`.
- **Alternativas**: primitivos de shell al estilo `op` para la IA (rechazado: le entrega el valor).

### D7. Niveles de confianza y `allow_execute` por proyecto

Tools ordenadas de menor a mayor poder: `list_secret_keys`/`get_context` (nombres) < `proxy_http_request` (sustitución acotada) < `execute_with_secrets` (RCE con secretos). La ejecución está **desactivada por defecto** y se habilita por proyecto.
- **Por qué**: `execute_with_secrets` es ejecución arbitraria con secretos, influenciable por prompt injection; el default debe ser el seguro.
- **Alternativas**: siempre activa (insegura), aprobación interactiva por llamada (MCP elicitation no universal; v2), allowlist de comandos (rígida).

### D8. Ejecución sin shell, con `shell` opcional

`execute_with_secrets` toma `command` + `args []string` y los ejecuta directamente (sin shell); existe un parámetro `shell` opcional para casos que realmente lo requieran.
- **Por qué**: evita inyección de shell y comporta igual en Win/Mac/Linux.
- **Alternativas**: un único string de shell (flexible pero peligroso). En Windows, `npm`/`npx` resuelven a `.cmd` vía PATHEXT y hay que validar el escapado de argumentos en `.cmd`/`.bat`.

### D9. Pipeline de redacción: normalizar → redactar (largo→corto) → truncar

Se captura stdout/stderr en bytes, se normaliza a UTF-8 (contemplando UTF-16LE de PowerShell 5.1), se sustituyen los valores por `[BLINDENV_REDACTED:<KEY>]` procesándolos de mayor a menor longitud, y se trunca si excede el límite. Umbral mínimo de longitud (por defecto 6) para no destrozar la salida con valores cortos; al guardar un secreto corto se avisa.
- **Por qué**: sin normalización previa el match exacto falla en Windows; sin orden por longitud quedan restos; sin umbral los valores cortos corrompen.
- **Alternativas**: redacción solo en memoria del modelo (insuficiente), whitelisting de salida (poco práctico).

### D10. Concurrencia con WAL y `busy_timeout`

SQLite en modo WAL, con `busy_timeout` configurado para tolerar varios procesos MCP y la UI.
- **Por qué**: el modelo multi-cliente implica escrituras concurrentes.
- **Alternativas**: servidor daemon único (complejidad y punto de fallo), locks a nivel de archivo (frágil).

### D11. UI local endurecida y embebida

Servidor HTTP en `127.0.0.1` con puerto configurable, token de sesión, validación estricta de `Host` y sin CORS. Frontend en HTML + JS vanilla con Tailwind compilado por CLI y embebido con `go:embed` (sin CDN, para respetar "local/sin red"). Modo `--dev` sirve desde disco.
- **Por qué**: un bind local no basta contra CSRF/DNS rebinding; el embed mantiene el binario único.
- **Alternativas**: framework SPA (peso innecesario), Tailwind por CDN (viola offline/CSP).

### D12. Backup portable cifrado con passphrase

Export/import a un archivo cifrado con passphrase, independiente del llavero de origen.
- **Por qué**: si desaparece la entrada del llavero, el vault es irrecuperable; el backup restore lo evita y permite mover el vault.
- **Alternativas**: confiar solo en el sistema operativo (inaceptable), export en claro (inaceptable).

### D13. Estructura de módulos

```
cmd/blindenv/main.go        -> CLI (mcp, ui, run, version)
pkg/crypto                  -> KeyProvider, AES-256-GCM, Argon2id
pkg/db                      -> SQLite (modernc), repositorio, migraciones, WAL
pkg/mcp                     -> servidor stdio, Tools, redaction engine, auditoría
pkg/web                     -> servidor HTTP local, API, assets embebidos
web/                        -> index.html, app.js, src.css (fuente) + dist/ (build)
```
- **Por qué**: separa cripto, persistencia, protocolo y presentación; el redaction engine vive junto al servidor MCP por proximidad al punto de salida.

## Risks / Trade-offs

- **La redacción es DLP, no sandbox**: un comando puede codificar, transformar o exfiltrar el valor. → Mitigación: documentar el threat model honesto; `allow_execute` off por defecto; preferir `proxy_http_request`; auditoría de uso.
- **Secretos cortos o comunes**: la redacción falla o corrompe la salida. → Mitigación: umbral mínimo y aviso al guardar.
- **Encoding en Windows (PS 5.1)**: deriva en fuga silenciosa si no se normaliza. → Mitigación: normalización previa y pruebas en Windows; preferir `pwsh`.
- **Escapado de argumentos en `.cmd`/`.bat`**: campo minado histórico. → Mitigación: pruebas dedicadas en Windows; documentar preferencia por `.exe`.
- **Pérdida del llavero**: vault irrecuperable. → Mitigación: backup portable (D12).
- **Prompt injection**: la IA puede intentar exfiltrar. → Mitigación: no imprimir valores, `proxy` como primitiva preferida, auditoría; riesgo residual documentado.
- **Build de Tailwind requiere Node**: añade dependencia de compilación. → Mitigación: pin del CLI, opción de CSS precompilado committeado.
- **Bloqueos SQLite bajo concurrencia**: errores intermitentes. → Mitigación: WAL + `busy_timeout` + reintentos.

## Migration Plan

Greenfield, sin datos previos. El esquema nace con una tabla `schema_version` para futuras migraciones. Rollback: al no haber usuarios, revertir el código deja el repo vacío sin consecuencias; los vaults creados en pruebas se descartan. Primer release objetivo: `v0.1.0`.

## Open Questions

- Valor exacto por defecto del umbral de redacción (6 u otro) y si se expone como configuración por proyecto.
- Si `run` debe soportar también un modo "dotenv" de salida para consumidores no-IA, o mantenerse estrictamente sin imprimir.
