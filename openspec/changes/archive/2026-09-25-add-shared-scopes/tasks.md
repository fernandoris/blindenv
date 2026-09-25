## 1. Storage and migration

- [x] 1.1 Rework `pkg/db/schema.go` to the scope-addressable `secrets` shape (nullable `project_id`, nullable `environment` name, no `environment_id` FK) with four partial unique indexes and `schemaVersion = 2`; verify with a test that creates one key in each of the four scopes for the same name.
- [x] 1.2 Implement the in-place v1 -> v2 migration inside a transaction: copy `secrets` joining `environments` for names, drop/rename, add `audit_log.key_scopes`; verify by opening a v1 fixture vault and asserting all pre-existing secrets resolve to the same values.
- [x] 1.3 Make `DeleteEnvironment` and `DeleteSecret` delete explicitly by `(project_id, environment)`; verify deleting an environment also removes its secrets (no orphans) in `pkg/db/db_test.go`.

## 2. Domain and resolution

- [x] 2.1 Extend `Scope` to four values and add provenance to `SecretInfo` in `pkg/db/models.go`; verify compile and that existing two-scope tests still pass after mapping.
- [x] 2.2 Rewrite `keySet` / `Resolve` / `ListKeys` / `ListSecrets` / `ScopeValues` / `PutSecret` in `pkg/db/store.go` to be scope-aware with the priority ladder (project+env > project-global > env-global > global); verify a table-driven test asserts the winner for every overlapping combination.
- [x] 2.3 Add a read-only effective-resolution accessor returning `key -> winning scope` for a project and environment (no values); verify it reports the correct source scope across all four tiers.
- [x] 2.4 Enforce the D3 environment-sharing rule (derived shared-environment set) in the store; verify the set includes both project environment names and names with only environment-global secrets.

## 3. Backup

- [x] 3.1 Extend `pkg/backup` export to include shared scopes and write a v2 backup; verify a round-trip export/import preserves global and environment-global secrets with their scopes.
- [x] 3.2 Make import accept v1 backups by mapping old project/environment ids to names; verify a v1 fixture imports into project + environment and project-global scopes correctly.

## 4. MCP and audit

- [x] 4.1 Verify `list_secret_keys`, `get_context`, `proxy_http_request` and `execute_with_secrets` return/act on the effective four-tier set; add a test proving a shared key is listed and injected for a project that does not define it.
- [x] 4.2 Persist and return the source scope per resolved key in the audit entry via `audit_log.key_scopes`; verify an entry records the shared scope when a key resolves from it and never records a value.

## 5. Web API

- [x] 5.1 Extend `/api/state` with a `shared` section (global and shared environments) and per-secret source scope; verify the JSON contains shared scopes and no values.
- [x] 5.2 Make reveal scope-qualified (accept the defining scope, not only environment) and update delete/create endpoints for shared scopes; verify revealing a shared secret and a project override return the correct values.
- [x] 5.3 Add an effective-resolution field to the state payload for a project and environment; verify it lists winning scopes without values.

## 6. Web UI shell

- [x] 6.1 Replace the stacked scope blocks with the master-detail shell: scope rail (Shared + collapsible Projects with counts and filter) and a single scope editor, reusing `el()`/`esc()`; verify only one editor renders at a time and projects collapse.
- [x] 6.2 Move the audit log into a header-triggered slide-over with project/environment/tool filters and focus management; verify the audit is closed on load and that filters narrow the rows.
- [x] 6.3 Add inline toasts and non-blocking errors, and surface the short-value warning from the create response; verify no `alert()` remains and a short value shows a warning.

## 7. Web UI shared scopes and inheritance

- [x] 7.1 Add the Shared section editors (global and each shared environment) and make the reveal cache scope-qualified; verify the same key name in different scopes reveals independently.
- [x] 7.2 Render inherited rows separately with source-scope labels and an `override` action that prefills the add form; verify an inherited key can be overridden in place and the new definition is marked as shadowing.
- [x] 7.3 Add the read-only effective view for a project/environment showing the winning scope per key; verify it matches `list_secret_keys` names and sources.
- [x] 7.4 Add destructive-action safeguards: typed confirmation with an impact list for shared/project-global deletes, and an affected-keys report for project/environment deletes; verify a shared delete lists affected scopes and requires the key name.
- [x] 7.5 Add reveal auto-hide plus copy, and accessibility attributes (`aria-current`, `aria-expanded`, `aria-pressed`, `<th scope>`, `aria-live`) without encoding tier by color alone; verify keyboard navigation and screen-reader state on the main flows.

## 8. Documentation

- [x] 8.1 Update `README.md`: four-scope model and precedence, dashboard description, backup compatibility and any CLI/environment notes; verify the scope examples match the implemented resolution.
- [x] 8.2 Update `CONTRIBUTING.md` where the artifact/scope workflow is described; verify it reflects the new scopes.
- [x] 8.3 Refresh the `vault` capability Purpose in `openspec/specs/vault/spec.md` after archive so it mentions the shared scopes; verify the text no longer implies only two scopes.

## 9. Integration verification

- [x] 9.1 Run `go test ./...`, `go vet ./...` and `CGO_ENABLED=0 go build ./...` and confirm all pass.
- [x] 9.2 Rebuild the Tailwind stylesheet with the documented command and confirm `web/dist/app.css` changes are committed.
- [x] 9.3 Run `openspec validate add-shared-scopes --strict` and confirm the change is valid.
