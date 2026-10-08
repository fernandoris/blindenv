## Why

When the master key does not match a vault — or a definition is marked sensitive without holding valid ciphertext — the system surfaces a per-key `crypto: encrypted data failed integrity check`. That single message collapses three distinct causes (wrong master key, a tampered or rotten value, and a mislabeled plaintext definition) into one ambiguous error. In a reproduced field report, a vault containing non-sensitive configuration values produced this error and the named key changed on every call, which reads like data corruption. The original trigger (a stale binary that decrypts *every* row because it predates the `sensitive` column) cannot be fixed inside old binaries; what the product can do is stop misreporting the cause and fail with an actionable diagnosis.

## What Changes

- State the storage invariant explicitly: a sensitive definition always stores a valid AES-256-GCM blob; a non-sensitive definition is never passed to decryption.
- Detect structurally impossible ciphertext (shorter than the AES-GCM nonce plus tag) and report it as an *inconsistent definition*, not a tamper/integrity error.
- Classify resolution failures: when a vault has at least one sensitive definition and none decrypts with the provided master key, report a *master-key mismatch*; when only some fail, report the specific unreadable definition.
- Verify the master key when a vault that contains sensitive definitions is unlocked, failing before any value is served when none decrypts, and stopping at the first successful decryption so the common case stays cheap.
- Keep the MCP server usable for discovery when the key does not match: `discover_secrets` and `list_secret_keys` continue to return names and metadata, while the value-consuming Tools return the classified error, so the agent sees the cause instead of a dropped connection.
- Keep the emitted errors actionable and free of any secret value.
- No vault format change and no migration.

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `vault`: adds decryption-failure classification and master-key verification at unlock, and strengthens the per-value encryption invariant (a sensitive value is always a valid AEAD blob; a non-sensitive value is never decrypted; impossible ciphertext is reported distinctly).
- `mcp-server`: adds a requirement that when the unlocked vault's key does not match, the value-consuming Tools report the classified cause while discovery and listing keep working.

## Impact

- `pkg/db/store.go` — resolution classifies decrypt failures and returns distinct sentinel errors; a verification step checks the master key when the vault has sensitive definitions; the structural length check happens before `crypto.Decrypt`.
- `cmd/blindenv/main.go` — `run` and `ui` fail fast on a key mismatch; `mcp` passes the verification result into the server so it can stay usable for discovery.
- `pkg/mcp` — the server carries the verification result and the value-consuming Tools return the classified error while discovery and listing are unaffected.
- `README.md` — troubleshooting distinguishes a per-definition integrity error from a master-key mismatch, and documents the MCP degraded behavior.
- Tests: `pkg/db` and `pkg/mcp` cover wrong-key, single-unreadable-definition, impossible-ciphertext, config-only and degraded-server cases; a config-only vault never invokes decryption.
- Performance and memory: unlock verification is benchmarked and given an allocation budget, with results recorded in a `performance-note.md`.
- No schema version bump; no change to `pkg/crypto`, the backup format, the MCP payload shapes or the web UI.

## Non-Goals

- Not changing the cryptography or the vault format.
- Not preventing older binaries from reading a newer vault (impossible to retrofit).
- No persistent key-check sentinel and no schema migration.
- No vault "written-by" stamp (marker-regression detection) and no `blindenv doctor` command — deferred until there is recurring evidence.
