## Context

See proposal.md - Why. Facts that shape the approach:

- Decryption happens in `pkg/db/store.go`. `ResolveDetailed` decrypts **every** effective sensitive value inside a `for key, r := range effective` loop and returns on the **first** error (`value()`, `pkg/db/store.go:458`). The message is `db: decrypt "<key>": crypto: encrypted data failed integrity check`.
- `crypto.Decrypt` (`pkg/crypto/cipher.go:36`) returns the single sentinel `ErrIntegrity` both when the input is shorter than the nonce (`len(data) < gcm.NonceSize()`, 12 bytes) and when AES-GCM authentication fails. It cannot, by itself, distinguish a wrong key from a corrupt value.
- `Open` (`pkg/db/store.go:95`) never verifies the master key; a mismatch is only discovered at the first decryption.
- Two key sources coexist: a global OS-keyring entry (`blindenv`/`master`) and a per-vault passphrase with a persisted salt. Mixing them produces a "wrong key" with no early signal.
- AES-256-GCM's minimum valid output is `nonce(12) + tag(16) = 28` bytes, so `len(enc) < 28` on a sensitive row is provably not a ciphertext.

## Goals / Non-Goals

**Goals:**
- Make a resolution failure name its cause: master-key mismatch vs a specific unreadable definition vs an inconsistent (structurally impossible) definition.
- Express the storage invariant so the class of bug that produced the report is testable.
- Stay schema-free: no migration, no format change.

**Non-Goals:**
- Sentinels, writer stamps, `doctor`, or any prevention of older binaries (see proposal Non-Goals).
- Changing the cryptography or the MCP payload shapes.

## Decisions

### D1. No schema change; classify at resolution

Classification is added at the resolution boundary, not by persisting any new state. `ResolveDetailed` already decrypts every effective sensitive value on each call, so the aggregate signal is available for free; only the abort-on-first-error behavior changes.

- Why: the reproduced failures are diagnosed from data already in hand. A schema migration is the highest-risk change in this project (it has its own compatibility incident) and buys nothing here.
- Alternative considered: a persistent per-vault key-check sentinel created at open. Rejected for this change — it needs a migration and has a chicken-and-egg problem (a sentinel cannot be created with an unverified key).

### D2. Sentinel errors in `pkg/db`, presentation upstream

Add `ErrMasterKeyMismatch` and `ErrInconsistentDefinition` alongside the existing per-key error. `pkg/mcp` and `cmd/blindenv` pass them through; they do not re-wrap in a way that loses the class.

- Why: the storage layer owns the diagnosis; the agent/CLI layer owns wording. This mirrors the existing split where `db` returns typed errors and `mcp` maps them to tool results.
- Existing `mcp-server` requirement "Errors without values" continues to hold — classification adds structure, never values.

### D3. Structural check before decryption

For a sensitive definition, if `len(enc) < 28`, return `ErrInconsistentDefinition` without calling `crypto.Decrypt`.

- Why: it is cheap, provable, and turns a cipher-level integrity error into a precise statement about the data. It also avoids feeding impossible input to the AEAD.
- The threshold is `gcm.NonceSize()+gcm.Overhead()`; it is derived from `pkg/crypto`, not a magic literal copied around.

### D4. Aggregate rule: all-fail means key mismatch, partial-fail means per-row

Collect failures across the effective sensitive definitions. If there is at least one sensitive definition and **all** fail, return `ErrMasterKeyMismatch`; if **some** succeed and some fail, return the specific failing definition. A vault with no sensitive definitions cannot fail this way.

- Why: a single wrong master key fails every row; a corrupt or mislabeled row fails alone. This is the distinction that turns the report's "random key" into a useful message.
- Alternative considered: probe only the first sensitive row. Rejected — one corrupt row would masquerade as a total mismatch.

### D5. Wording does not overclaim

The mismatch message says no sensitive value could be read with the current master key and lists the likely causes (different passphrase or keyring, or a vault from another installation/version). It does not assert "wrong key".

- Why: total corruption would produce the same aggregate signal; the message must stay honest while still pointing at the common cause.

### D6. Verify the master key at unlock, stopping at the first success

Add a verification the command layer runs after `db.Open`: when the vault has at least one sensitive definition, decrypt until the first success; if none decrypts, return `ErrMasterKeyMismatch`. Rows are streamed from SQLite, so the matching-key case reads and decrypts exactly one row, and memory does not grow with the number of definitions.

- Why: a wrong key should fail before any value is served, but the common case must stay cheap. Stopping at the first success makes the hot path one decryption; the full scan is paid only when every definition fails.
- Alternative considered: always decrypt every sensitive value at unlock. Rejected — needless work for a matching key.
- Alternative considered: sample a single definition. Rejected — a single corrupt definition would produce a false mismatch.

### D7. Keep the MCP server usable for discovery under a mismatch

The command layer passes the verification outcome into the MCP server. Under a mismatch, `discover_secrets` and `list_secret_keys` keep working and the value-consuming Tools return the classified error instead of the process exiting.

- Why: failing the process makes the MCP client show a dropped connection and hides the diagnosis from the agent — the exact confusion this change removes. Key names and metadata are already public, so serving discovery without the key exposes nothing new.
- Alternative considered: hard-fail the process on mismatch, like the schema guard. Rejected here because it trades an agent-visible, actionable error for "connection closed".

## Risks / Trade-offs

- [All rows genuinely corrupt, reported as a likely key mismatch] -> Wording lists key mismatch as the likely cause, not a certainty, and is still actionable.
- [Classifying requires attempting every sensitive value on the failure path] -> No new cost: `ResolveDetailed` already decrypts all of them today; the success path is unchanged.
- [Distinct errors could reveal which keys exist] -> Key names are already public via `list_secret_keys`; no values are added.
- [Older binaries never benefit] -> Out of scope; covered operationally by build identity and the README note.
- [Unlock verification adds one decryption to every open on the hot path] -> It stops at the first success; a benchmark pins it as negligible next to SQLite open and migration probes.
- [The mismatch path reads every sensitive row] -> Rows are streamed and decrypted one at a time, so memory stays bounded; only the failure path pays it.
- [Serving discovery without a matching key] -> Only names and metadata, which `list_secret_keys` already returns; no values.

## Migration Plan

None. No schema version bump and no format change. Only the failure-path behavior changes, so rollback is a plain revert with no data implications.

## Performance and Memory Verification

The repo records performance evidence before archiving (benchmarks in `pkg/db/perf_test.go`, allocation budgets via `testing.AllocsPerRun`, and a `performance-note.md` in the change directory). This change measures the new unlock-verification hot path:

- **Matching key**: `BenchmarkUnlockVerifyMatching` over a seeded vault, to show the added cost is a single AES-GCM decryption.
- **Mismatch**: `BenchmarkUnlockVerifyMismatch` (full scan) to bound the failure path.
- **Steady state**: compare against opening an already-current vault so the single decrypt is visibly negligible versus SQLite open and migration probes.
- **Bounded memory**: `testing.AllocsPerRun` asserting the matching-key verification does not allocate proportionally to the number of definitions (rows streamed, stopped at first success).
- Record the numbers and the conclusion in `openspec/changes/diagnose-decrypt-failures/performance-note.md` before archiving.

## Open Questions

None.
