# Tasks

## 1. Store update operation

- [x] 1.1 Add `UpdateSecret(ctx, project, environment, key string, value *string, meta *SecretMeta) (short bool, error)` in `pkg/db/store.go`: resolve the scope, read the existing row in a transaction, return `ErrSecretNotFound` when absent, merge supplied fields over the stored values, and delegate the write to `writeSecret`. Verify with new unit tests that pass under `go test ./pkg/db`.
- [x] 1.2 Cover identity and representation: an update preserves scope, key and `created_at`, advances `updated_at`, re-encrypts a sensitive value with a different ciphertext, keeps a non-sensitive value in cleartext, and re-encodes when sensitivity flips. Verify with a store test that reads the raw columns.
- [x] 1.3 Reject an explicitly empty replacement value while allowing an omitted (`nil`) value to keep the stored one. Verify with tests for both cases.
- [x] 1.4 Validate the hint against the effective value whether the hint is supplied or preserved, rejecting with `ErrInvalidMetadata` and leaving the row unchanged. Verify with tests for both cases.
- [x] 1.5 Return the short-value flag when the effective value is below `MinSecretLength`. Verify with a test asserting the returned flag and the stored value.
- [x] 1.6 Prove an update cannot create or edit a broader scope: updating a key absent from the selected scope fails with `ErrSecretNotFound` and leaves any broader definition untouched. Verify with a store test.
- [x] 1.7 Extend the concurrent-writer coverage so simultaneous `UpdateSecret` calls on separate `Store` handles complete without error or corruption. Verify with a test in the style of `TestManyConcurrentWriters`.
- [x] 1.8 Add `BenchmarkSecretValueUpdate` and a bounded-allocation test comparing a small and a large vault in `pkg/db/perf_test.go`, showing update cost does not scale with definition count. Verify with `go test ./pkg/db -run '^$' -bench BenchmarkSecretValueUpdate -benchmem` and the allocation test.

## 2. Web API

- [x] 2.1 Extend the project and shared `PATCH` bodies with an optional `value *string` and route the call to `UpdateSecret`; return success plus the short-value flag and never return the value. Verify with `httptest` tests in `pkg/web/server_test.go`.
- [x] 2.2 Test PATCH with a replacement value and without one (metadata-only), for both project and shared scopes, asserting the value changes or is preserved as appropriate. Verify with the new API tests.
- [x] 2.3 Assert the response body never contains the value, and that editing a missing key returns a client error without creating a definition. Verify with the new API tests.
- [x] 2.4 Keep the existing metadata-only PATCH tests green as a regression check. Verify with `go test ./pkg/web`.

## 3. Dashboard

- [x] 3.1 Replace the metadata-only dialog with an "Edit secret" dialog that adds an optional replacement-value input with an accessible label and a description stating that blank keeps the current value. Verify by inspecting the rendered dialog in `blindenv ui`.
- [x] 3.2 Wire save to the existing `PATCH` request, omitting `value` when the input is blank, showing inline success and error feedback, and surfacing the short-value warning after save. Verify manually against a temporary vault: value change, blank-value keep, combined value+metadata change.
- [x] 3.3 Rebuild the committed stylesheet only if new Tailwind utility classes are introduced, using the command in `CONTRIBUTING.md`, and verify `web/dist/app.css` is updated.

## 4. Documentation and compatibility

- [x] 4.1 Update `README.md` to describe in-place secret editing in the dashboard feature list and state that agents still cannot edit secrets. Verify by reading the updated sections.
- [x] 4.2 Confirm the change adds no schema column, schema-version bump, backup-format change, MCP tool or CLI subcommand. Verify with `git diff -- pkg/db/schema.go pkg/backup pkg/mcp cmd` showing no functional change.
- [x] 4.3 Verify historical vaults still open and can be edited by adding a compatibility test that seeds an older schema, opens it, and updates a value. Verify with `go test ./pkg/db -run Compat`.
- [x] 4.4 Review every added spec, comment and document for English-only text. Verify with a final diff read.

## 5. Integration verification

- [x] 5.1 Run `make check` (gofmt gate, `go vet`, `go build`, `go test`) and fix any failure.
- [x] 5.2 Re-run the benchmark and bounded-allocation test on the final code and record the results in `performance-note.md` in this change directory.
- [x] 5.3 Exercise the full flow against a temporary vault: create, edit value without reveal, edit metadata only, edit value plus metadata, and confirm resolution returns the latest value.

## Workflow follow-up

- Archive the change after the project's review requirements are satisfied.
- Verify the archived specs and tasks before finishing.
