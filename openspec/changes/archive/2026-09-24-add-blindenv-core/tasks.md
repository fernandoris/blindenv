## 1. Bootstrap del proyecto

- [x] 1.1 Inicializar el módulo Go (`go.mod`) y la estructura `cmd/blindenv`, `pkg/{crypto,db,mcp,web}`, `web/`; verificar que `go build ./...` compila el esqueleto vacío
- [x] 1.2 Añadir dependencias (`mark3labs/mcp-go`, `modernc.org/sqlite`, `zalando/go-keyring`, `x/crypto/argon2`) y verificar que `go mod tidy` no introduce CGO
- [x] 1.3 Configurar CI con matriz macOS/Windows/Linux y verificar que `go test ./...` corre en las tres plataformas

## 2. Núcleo criptográfico (`pkg/crypto`)

- [x] 2.1 Definir la interfaz `KeyProvider` (32 bytes) y verificar con un provider de prueba en memoria
- [x] 2.2 Implementar `OSKeyring` con `go-keyring` (servicio `blindenv`, entrada `master`) y verificar lectura/escritura en macOS y Windows
- [x] 2.3 Implementar `PassphraseProvider` con Argon2id y salt persistido; verificar que la misma passphrase + salt deriva la misma clave
- [x] 2.4 Implementar `Encrypt`/`Decrypt` AES-256-GCM con nonce aleatorio; verificar test de round-trip y test de detección de manipulación
- [x] 2.5 Verificar que el mismo valor cifrado dos veces produce ciphertexts distintos (test unitario)

## 3. Persistencia (`pkg/db`)

- [x] 3.1 Crear el esquema (proyectos, entornos, secretos con `environment` nullable, `schema_version`) y verificar migración inicial en base limpia
- [x] 3.2 Activar WAL y `busy_timeout`; verificar lectura/escritura concurrente con la UI mediante test con dos conexiones
- [x] 3.3 Implementar el repositorio de proyectos/entornos/secretos con unicidad `(proj,NULL,key)` y `(proj,env,key)`; verificar rechazo de duplicados en test
- [x] 3.4 Implementar la resolución de valor efectivo (entorno pisa global) y verificar los tres escenarios de la spec `vault`
- [x] 3.5 Implementar el aviso de secreto por debajo del umbral de redacción; verificar que devuelve la advertencia al guardar un valor corto

## 4. Servidor MCP (`pkg/mcp`)

- [x] 4.1 Arrancar el servidor `stdio` con `mcp-go` y verificar que los logs van a stderr y stdout solo lleva protocolo
- [x] 4.2 Implementar `list_secret_keys` (unión global+entorno, solo nombres) y verificar que la respuesta no contiene valores
- [x] 4.3 Implementar `get_context` (os, arch, shell_hint, proyecto, entorno, nombres) y verificar la ausencia de valores
- [x] 4.4 Implementar la resolución de contexto desde config con override por llamada y verificar ambos escenarios
- [x] 4.5 Implementar el Redaction Engine (normalización UTF-8 → sustitución largo→corto → umbral mínimo → truncado) y verificar con tests de encoding UTF-16 y valores solapados
- [x] 4.6 Implementar `execute_with_secrets` sin shell (con `shell` opcional), inyección de entorno, captura de salida y redacción; verificar rechazo si `allow_execute` está desactivado
- [x] 4.7 Implementar `proxy_http_request` con sustitución `{{SECRET_NAME}}` JSON-aware y limpieza de auth en redirects cross-host; verificar con servidor HTTP de prueba
- [x] 4.8 Implementar el registro de auditoría (nombres, comando, exit code, nº redacciones; sin valores) y verificar que una ejecución genera su entrada

## 5. CLI (`cmd/blindenv`)

- [x] 5.1 Implementar la resolución de la ruta del vault (`os.UserConfigDir()` + override) y verificar en las tres plataformas
- [x] 5.2 Implementar `mcp`, `ui`, `run` y `version`, y verificar `blindenv --help` y el error de subcomando desconocido
- [x] 5.3 Implementar `run` (inyección sin imprimir + propagación de exit code) y verificar que un comando hijo con salida no nula propaga su código
- [x] 5.4 Verificar que no existe ningún subcomando que imprima valores de secretos en stdout

## 6. UI web local (`pkg/web` + `web/`)

- [x] 6.1 Configurar el build de Tailwind (`npx tailwindcss`) y verificar que `web/dist/app.css` se genera y se embebe con `go:embed`
- [x] 6.2 Levantar el servidor HTTP en `127.0.0.1` con puerto configurable, token de sesión y validación de `Host`; verificar rechazo sin token y con `Host` inesperado
- [x] 6.3 Implementar la API de CRUD de proyectos/entornos/secretos y verificar alta y listado sin exponer valores
- [x] 6.4 Implementar el revelado explícito de un secreto y verificar que los listados nunca muestran valores
- [x] 6.5 Implementar la marca de `override` (entorno sobre global) y verificar en la vista
- [x] 6.6 Implementar el interruptor `allow_execute` por proyecto y verificar que su cambio afecta a las invocaciones de la Tool
- [x] 6.7 Implementar la vista de auditoría (sin valores) y verificar que muestra las entradas registradas
- [x] 6.8 Verificar el modo `--dev` (sirve assets desde disco) frente al modo embebido

## 7. Backup portable

- [x] 7.1 Implementar la exportación cifrada con passphrase (UI/CLI) y verificar la generación del archivo
- [x] 7.2 Implementar la importación con passphrase y verificar round-trip en equipo distinto y el rechazo con passphrase incorrecta sin alterar el vault

## 8. Documentación de proyecto libre

- [x] 8.1 Escribir `README.md` (inglés) con propósito, features e instalación por plataforma; verificar que un lector puede instalar y arrancar el binario siguiendo solo el README
- [x] 8.2 Añadir la sección de integración con fragmentos listos para `opencode.jsonc` (`mcp.local`), `claude_desktop_config.json`/`.mcp.json` y `.cursor/mcp.json`, bajo la clave `"blindenv"`
- [x] 8.3 Documentar el **threat model honesto** (la redacción es DLP, no sandbox; riesgo de exfiltración por comando; `allow_execute` off por defecto) y verificar que el README lo declara explícitamente
- [x] 8.4 Añadir `LICENSE` (OSI aprobada), guía de contribución mínima y cómo ejecutar tests/build; verificar que los archivos existen y enlazan desde el README

## 9. Verificación de integración

- [x] 9.1 Ejecutar un servidor MCP real contra los cuatro Tools y verificar el flujo completo (contexto → list → execute → salida redactada) en macOS, Windows y Linux
- [x] 9.2 Verificar el escenario de concurrencia real: UI escribiendo mientras varios procesos MCP operan sin corrupción
- [x] 9.3 Verificar el cross-compile de los tres objetivos (`GOOS=windows/darwin/linux`) sin CGO
