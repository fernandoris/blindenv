## 1. Storage and schema

- [x] 1.1 Add schema v3 columns `sensitive INTEGER NOT NULL DEFAULT 1`, `kind TEXT NOT NULL DEFAULT ''`, `hint TEXT NOT NULL DEFAULT ''`, `value_plain TEXT` to `secrets` via additive `ALTER TABLE`, guarded by `columnExists`; verify with a db test that opens a v2 vault, migrates it, and preserves existing values (existing `pkg/db` tests still pass)
- [x] 1.2 Define the `SecretMeta` model and the closed type enum (`url`, `host`, `connection-string`, `token`, `password`, `text`), with validation: unknown type rejected, empty type defaults to `text`, `token`/`password` forced sensitive, hint bounded (200 chars) and rejected when it contains the value; verify with table-driven unit tests in `pkg/db`
- [x] 1.3 Implement `PutSecretMeta` (stores `value_enc` for sensitive and `value_plain` for non-sensitive, enforcing exactly-one-set) and keep `PutSecret` as a thin default wrapper; verify store tests cover both paths and the XOR invariant
- [x] 1.4 Implement `SetSecretMetadata` (metadata-only update, re-encoding only when sensitivity flips) and verify tests that flipping sensitive<->config preserves the value and holds the XOR invariant

## 2. Resolution and repository APIs

- [x] 2.1 Extend `effectiveSecrets` to read and carry `kind`, `hint`, `sensitive` and `value_plain` through candidates and the winning definition; verify existing resolution tests plus new tests asserting the winner's metadata
- [x] 2.2 Add a detailed resolution API returning `value`, `sensitive`, `kind`, `hint`, `scope` and `overrides` per key, keeping `Resolve`/`ResolveKey` as value-only wrappers; verify unit tests for sensitive and non-sensitive keys in all four scopes
- [x] 2.3 Extend `SecretInfo`, `ListSecrets` and `ScopeSecrets` to include `kind`, `hint` and `sensitive` without values; verify db tests
- [x] 2.4 Add a per-scope entries API returning value plus metadata for backup; verify a db test that reads back metadata for a mixed scope

## 3. MCP exposure and redaction

- [x] 3.1 Include `type`, `hint` and `sensitive` on every key in `get_context` and return the value only for non-sensitive keys; verify `pkg/mcp` server tests assert metadata presence, sensitive value withheld, non-sensitive value returned
- [x] 3.2 Include `type`, `hint` and `sensitive` on every key in `discover_secrets`, still returning no values (including for non-sensitive keys); verify server tests
- [x] 3.3 Build the redactor from sensitive values only while injecting all resolved values in `proxy.go` and `exec.go`; verify exec/proxy tests assert a non-sensitive value appears unchanged and a sensitive value is redacted
- [x] 3.4 Confirm `list_secret_keys` remains names-only and unchanged; verify the existing test still passes unmodified

## 4. Web API and UI

- [x] 4.1 Add `type`, `hint` and `sensitive` to `secretView`/`effectiveView` and populate them in `handleState`; verify `pkg/web` server tests assert metadata is present and non-sensitive values are returned only where allowed
- [x] 4.2 Add `PATCH /api/projects/<slug>/secrets` and `PATCH /api/shared/secrets` for metadata-only edits; verify server tests create, patch metadata without a value, and read back the change
- [x] 4.3 Extend the add form with type, a `Secret`/`Config` radio and an optional hint, plus the "stored unencrypted and visible to agents" warning when `Config` is chosen; verify by exercising the form against a running `blindenv ui --dev`
- [x] 4.4 Render rows with a muted type label, an expandable accessible hint, and non-sensitive values shown without a reveal action or auto-hide; verify in the dashboard for both a sensitive and a non-sensitive key
- [x] 4.5 Add an edit action that opens the form in metadata-only mode (value untouched) and prefill metadata from the inherited definition when using override; verify the hint survives an override round trip created from the UI
- [x] 4.6 Extend the filter to match key name, type and hint; verify typing a hint-only fragment and a type name narrows the list
- [x] 4.7 Ensure the non-sensitive marker is conveyed by text/icon and not color alone, and that the hint is keyboard-reachable with `aria-expanded`; verify by keyboard-only navigation and an accessibility check

## 5. Backup

- [x] 5.1 Extend export with a sibling `Metadata` map per scope (`omitempty`) and import that preserves type/hint/sensitivity, defaulting missing metadata to sensitive with no type/hint; verify `pkg/backup` tests round-trip metadata and import existing V1/V2 fixtures unchanged

## 6. Documentation

- [x] 6.1 Update `README.md` (features and threat model) to state that non-sensitive configuration values are stored unencrypted, are not redacted, and are visible to agents, and to describe type/hint; verify by review

## 7. Performance tests

- [x] 7.1 Add `BenchmarkEffectiveSecrets` covering many keys across all four scopes with mixed sensitivity, capture a baseline on the current code, and report `benchstat` before/after
- [x] 7.2 Add `BenchmarkRedactorMixedSensitivity` and confirm building the redactor from sensitive-only values does not regress substitution throughput
- [x] 7.3 Add `BenchmarkListScopedKeys` over a project with many environments and keys, and confirm the extra metadata columns do not change the N+1 query profile asymptotically
- [x] 7.4 Add `BenchmarkExportImport` with mixed metadata and confirm the metadata map does not materially change backup round-trip time
- [x] 7.5 Record the benchmark results and `benchstat` comparison as the performance artifact for the change; verify the numbers are attached to the change before archiving

## 8. Validation

- [x] 8.1 Run `go test ./...`, `go vet ./...` and `CGO_ENABLED=0 go build ./...`; verify all pass
- [x] 8.2 Run `openspec validate add-secret-key-metadata --strict`; verify the change validates
- [x] 8.3 Perform an end-to-end check: create a non-sensitive `url` key with a hint for `RANCHER_API_ENDPOINT`, call `get_context` and confirm the value and hint are returned, then run a command that prints the value and confirm it is not redacted
