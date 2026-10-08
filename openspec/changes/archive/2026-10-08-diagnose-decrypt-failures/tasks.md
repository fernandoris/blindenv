## 1. Invariant and structural detection

- [x] 1.1 Add a structural check in the store's value resolution: a sensitive definition whose stored ciphertext is shorter than the AES-GCM nonce plus tag length is classified as `ErrInconsistentDefinition` before any decryption; verify a unit test inserts such a row and asserts the classified error, not an integrity error.
- [x] 1.2 Derive the minimum length from `pkg/crypto` (nonce size plus tag/overhead) rather than a copied literal; verify the constant is exported or referenced so the check and `Decrypt` cannot drift.
- [x] 1.3 Prove non-sensitive definitions never reach decryption: resolve a vault whose only definition is non-sensitive using a different master key and verify it succeeds without error.

## 2. Failure classification

- [x] 2.1 Add sentinel errors `ErrMasterKeyMismatch` and `ErrInconsistentDefinition` in `pkg/db`; verify they are returned by the resolution path and are matchable with `errors.Is`.
- [x] 2.2 Change resolution to collect decrypt failures instead of aborting on the first: when at least one sensitive definition is effective and all fail, return `ErrMasterKeyMismatch`; when some succeed and some fail, return the specific failing definition; verify tests for the all-fail, partial-fail and no-sensitive cases.
- [x] 2.3 Confirm a config-only vault resolves for any master key and that `list_secret_keys` / `discover_secrets` remain unaffected (no decryption); verify an MCP-level test still lists names while `get_context` and `execute_with_secrets` report the mismatch.
- [x] 2.4 Ensure every classified error message is actionable (names the likely cause for a mismatch; names the definition for a per-key failure) and contains no secret value; verify a test asserts the mismatch text and that no stored value appears.

## 3. Unlock verification and MCP degraded mode

- [x] 3.1 Add a store verification that streams sensitive definitions and decrypts until the first success, returning `ErrMasterKeyMismatch` when none decrypts; verify unit tests for a matching key (stops at first success), a total mismatch, and a vault with no sensitive definitions.
- [x] 3.2 Wire verification into the command layer: `blindenv run` and `blindenv ui` fail fast with the classified message; verify a wrong-passphrase vault exits with the mismatch error.
- [x] 3.3 Pass the verification outcome into the MCP server so discovery keeps working; verify `list_secret_keys` / `discover_secrets` succeed and `get_context` / `execute_with_secrets` / `proxy_http_request` / `open_in_browser` return the mismatch error without side effects under a non-matching key.
- [x] 3.4 Ensure the verification streams rows so memory does not grow with the number of definitions; verify with a `testing.AllocsPerRun` budget on the matching-key path.

## 4. Performance and memory

- [x] 4.1 Add `BenchmarkUnlockVerifyMatching` and `BenchmarkUnlockVerifyMismatch` over a seeded vault; verify the matching-key case performs a single decryption and report the numbers.
- [x] 4.2 Add an allocation budget asserting the matching-key unlock verification does not allocate proportionally to vault size; verify `testing.AllocsPerRun` stays under the ceiling.
- [x] 4.3 Write `openspec/changes/diagnose-decrypt-failures/performance-note.md` with the unlock cost, the allocation figures and the conclusion; verify the file exists and matches the recorded `go test -bench` output.

## 5. Documentation

- [x] 5.1 Extend the `README.md` troubleshooting note to distinguish a per-definition integrity error from a master-key mismatch and point to `blindenv version` and the MCP client's passphrase/keyring; verify by review.
- [x] 5.2 Document the MCP degraded behavior (discovery keeps working, value-consuming Tools report the mismatch) in `README.md`; verify by review.

## 6. Verification

- [x] 6.1 Run `make check`; verify gofmt, `go vet`, build and the full test suite pass.
- [x] 6.2 End-to-end: resolve a config-only vault (succeeds), a wrong-passphrase vault (master-key mismatch), and a vault with a structurally short sensitive row (inconsistent definition), confirming each message is distinct and carries no values.
- [x] 6.3 End-to-end MCP: under a non-matching key, `list_secret_keys` returns names while `execute_with_secrets` returns the mismatch; capture the transcript.
