## 1. Vault: allow_open capability

- [x] 1.1 Add an additive `allow_open` boolean column (default `0`) to the projects schema and a migration that sets it disabled for existing rows; verify a pre-change vault opens and every project reports `allow_open == false`
- [x] 1.2 Add `AllowOpen` to the project model and read/write it in the store (create + update), mirroring `AllowExecute`; verify a unit test round-trips both flags independently
- [x] 1.3 Verify a backup export/import round trip preserves `allow_open` (or documents that it is restored as disabled) with a db test

## 2. MCP: open_in_browser tool

- [x] 2.1 Implement `pkg/mcp/open.go`: resolve `{{SECRET_NAME}}` with the existing `substitute` helper, gate on `allow_open`, refuse an empty environment, and refuse on headless Linux (`DISPLAY`/`WAYLAND_DISPLAY` empty); verify unit tests for each refusal
- [x] 2.2 Invoke the OS launcher without a shell (`open` / `xdg-open` / `rundll32 url.dll,FileProtocolHandler`) and return only "handed to the launcher"; verify with a fake launcher that no shell is used and the URL is a single argument
- [x] 2.3 Audit only the unsubstituted URL template plus key names and source scopes; verify a test asserts the resolved value appears in no audit entry, result or error message
- [x] 2.4 Register `open_in_browser` in `registerTools` and add the tool schema (`url` required, optional `project`/`environment`); verify the tool is listed and callable through the MCP server test harness
- [x] 2.5 Include `allow_open` in the `get_context` response and add its scenario test

## 3. Web UI: permission control

- [x] 3.1 Expose the `allow_open` toggle through the project update endpoint; verify an API test toggles it and `allow_execute` is unchanged
- [x] 3.2 Add the `allow_open` control to the project view with a note that it lets an agent hand a resolved URL to the local browser; verify manually in `blindenv ui --dev`

## 4. Documentation

- [x] 4.1 Update README: tool list, feature list, and CLI/tool usage for `open_in_browser`
- [x] 4.2 Add the threat-model bullets: `ps` argv window, browser-history persistence, and read-back via agent browser automation; verify the caveats read as warned-against, not protected-against

## 5. Verification and performance

- [x] 5.1 Add a security test asserting the resolved URL never reaches the agent, the audit log, any error, or the returned payload, and that argv is shell-free; verify it passes
- [x] 5.2 Run `go test ./...`, `go vet ./...`, `CGO_ENABLED=0 go build ./...` and confirm all green
- [x] 5.3 Add a benchmark or measurement for the open path (in-process substitution + launcher spawn) and record the memory profile; confirm the work is O(URL length) with no output buffering or redaction pass, and document why a micro-benchmark is not the meaningful test here
