## Context

See `proposal.md` for motivation. The relevant current state:

- `secrets` rows are scope-addressable definitions: a NULL `project_id` means shared, a NULL `environment` means all environments (`pkg/db/schema.go`).
- `effectiveSecrets` (`pkg/db/store.go`) collects every applicable candidate for a key and picks the winner; `Resolve` decrypts the winner's `value_enc` into `map[string]string`; `proxy.go` and `exec.go` build a `Redactor` from that map.
- `SecretInfo` (`pkg/db/models.go`) carries `Key`, `Scope`, `Environment`, `Overrides` and never a value.
- The backup format (`pkg/backup/backup.go`) stores values as `map[string]string` per scope, tagged `magicV1`/`magicV2`.
- The web UI renders every value masked (`web/app.js`), and the API exposes `secretView`/`effectiveView` without metadata.

## Goals / Non-Goals

**Goals:**

- Let a definition describe *what it is* (type) and *how to use it* (hint), so agents stop guessing.
- Let a definition be marked non-sensitive so its value is readable at rest, never redacted, and returned by `get_context`.
- Keep the four-scope model, resolution and redaction contract intact for sensitive values.
- Keep old backups importable and old behavior as the default.

**Non-Goals:**

- Metadata inheritance across scopes. A definition's type/hint/sensitivity are its own; an override does not inherit the broader definition's hint. (The UI mitigates this by prefilling metadata when overriding; see Decisions.)
- A general key-value config store or nested/structured metadata. `type` and `hint` are intentionally small.
- Changing `list_secret_keys`: it stays names-only.
- Changing the redaction algorithm, its memory bounds or its normalization.

## Decisions

### D1: Metadata lives on the definition row

Add `sensitive`, `kind`, `hint` (and `value_plain`) to `secrets`. The winning definition supplies the effective metadata, symmetric with how it supplies the value.

- **Why**: each scope row is already an independent definition and the resolver already selects a winner. Keeping metadata on the row means no join and no second resolution pass.
- **Alternatives considered**: a vault-wide `secret_meta(key, kind, hint)` table (one hint per key name). It matches "per key" literally and makes a hint survive overrides, but it forces one metadata for the same name across unrelated projects and splits sensitivity (per value) from type/hint (per name) across two models. Rejected for v1; can be revisited if cross-scope hint survival proves important.

### D2: `sensitive` is a per-definition flag that changes storage, redaction and visibility

```
  sensitive = true  (default)        sensitive = false
  +--------------------------+       +--------------------------+
  | value_enc  = ciphertext  |       | value_plain = cleartext  |
  | redacted from output     |       | returned verbatim        |
  | never returned to agent  |       | returned by get_context  |
  +--------------------------+       +--------------------------+
```

`value_plain` is nullable and holds the cleartext for a non-sensitive definition. `value_enc` stays `NOT NULL` (an additive `ALTER TABLE` cannot drop it), so a non-sensitive row stores an empty blob there. The store enforces the invariant: `value_enc` carries ciphertext iff `sensitive` is true, and `value_plain` carries cleartext iff `sensitive` is false.

- **Why**: the requirement is that configuration is usable without a keyring and readable at rest, which encryption cannot provide. `value_plain` makes "readable at rest" explicit in the schema instead of mixing plaintext and ciphertext in one column behind a flag.
- **Alternatives considered**: (a) always encrypt and only skip redaction/visibility — preserves the threat model but cannot be read without the keyring; rejected because it does not meet the stated need; (b) reuse `value_enc` to hold cleartext when non-sensitive — a type-confusion foot-gun where a missing flag makes ciphertext and cleartext indistinguishable; rejected.
- **Risk accepted**: a non-sensitive value is exposed to anyone who can read the database file. This is a deliberate, documented threat-model change (see Risks).

### D3: Closed type enum with a sensitivity constraint

`url`, `host`, `connection-string`, `token`, `password`, `text`. `token` and `password` must be sensitive; the other types may be either. Empty type defaults to `text`.

- **Why**: a closed enum is testable and lets the UI/API drive validation, iconography and default sensitivity. Free-form types would be untestable and would not let the system reason about sensitivity.
- **Alternatives considered**: free-form type string. Rejected: no validation value, unclear ownership of allowed values.

### D4: `hint` never contains the value; validated on write

On create/update (and on metadata-only patch, by reading the stored value in-process), reject a hint that contains the definition's value.

- **Why**: the hint is returned to agents and shown in the UI. Allowing it to carry the value would turn a convenience field into a leak channel.
- **Trade-off**: a legitimate hint that quotes a full value is refused. Acceptable; hints are descriptive, not value-bearing.

### D5: Resolution carries metadata; the redactor stays value-only

Introduce a detailed resolution result (`value`, `sensitive`, `kind`, `hint`, `scope`, `overrides`). `proxy.go`/`exec.go` inject every resolved value but build the `Redactor` from **sensitive values only**; `NewRedactor(map[string]string, int)` is unchanged.

- **Why**: the redaction engine should not know about types or sensitivity — it just receives a smaller input set. This keeps the bounded/ordered substitution properties and their tests intact.
- **Alternative considered**: pass sensitivity into the redactor and let it filter. Rejected: spreads policy into the substitution core.

### D6: Metadata-only edits use a dedicated endpoint

Add `PATCH /api/projects/<slug>/secrets` and `PATCH /api/shared/secrets` accepting `environment`, `key`, `type`, `hint`, `sensitive` (no value). The store re-encodes only when sensitivity flips.

- **Why**: otherwise the UI must reveal the value just to label it, which contradicts the "explicit reveal" requirement and needlessly exposes values in the browser. It also makes "edit the hint" a one-field operation.
- **Alternative considered**: reuse `PUT`-with-value for all edits. Rejected: forces value re-entry/reveal.

### D7: Backup stays value-map based, with a sibling metadata map

Keep `Secrets map[string]string` and add `Metadata map[string]meta` (`meta{Sensitive *bool, Type, Hint}`) per scope, `omitempty`. `Sensitive == nil` means `true`.

- **Why**: backwards compatible in both directions. Old backups lack `Metadata`, so definitions import as sensitive with no type/hint. Old binaries ignore the unknown `Metadata` field. Changing `Secrets` to a struct-per-entry would break `map[string]string` unmarshalling of V1/V2 files.
- **Alternative considered**: a `magicV3` format with a new shape. Rejected: more code and a decode fork for no added safety, since the backup is already passphrase-encrypted.

### D8: Schema migration v3 is additive

`ALTER TABLE secrets ADD COLUMN` for `sensitive INTEGER NOT NULL DEFAULT 1`, `kind TEXT NOT NULL DEFAULT ''`, `hint TEXT NOT NULL DEFAULT ''`, `value_plain TEXT`. The XOR invariant is enforced in the store, not by a SQL CHECK (SQLite cannot add a CHECK via `ALTER TABLE` without a table rebuild).

- **Why**: additive migration is the least risky and matches the existing migration style.
- **Alternative considered**: rebuild the table (as `migrateSecretsV1` does) to attach a CHECK. Deferred; the invariant is small and localized in the store.

### D9: Sensitive is explicit in the UI, never implied silently

The add/edit form starts at `Secret` for every type. Choosing `url`/`host` surfaces a suggestion to switch to `Config`, but storage is plaintext only when the user selects it and acknowledges the warning.

- **Why**: defaulting to plaintext based on type would silently weaken the threat model for the most common configuration keys.
- **Alternative considered**: auto-select `Config` for `url`/`host`. Rejected: accidental exposure.

## Risks / Trade-offs

- [Threat model weakens for non-sensitive values: readable at rest, no redaction] -> Document it in the README and the UI warning; constrain `token`/`password` to sensitive; keep the default sensitive; keep backups encrypted with the passphrase.
- [Downgrade hazard: an older binary cannot read `value_plain` rows] -> Document that upgrades are forward-only for non-sensitive data and that a pre-upgrade backup is the rollback path; do not auto-downgrade.
- [Hint becomes a leak channel] -> Reject hints containing the value (D4); never log hints with values; hints are metadata, not secrets, so they may appear in `get_context` and the UI.
- [Chip overload in the UI collapses scope/type/sensitivity signals] -> Type is muted monospace text, not a colored chip; sensitivity is marked only when non-default and never by color alone (`web-ui` spec).
- [Migration cost: extra columns in the hottest read path] -> The extra columns are small and returned in the same query; measured by the benchmarks in `tasks.md`.
- [Override drops the hint (no inheritance, D1)] -> The UI prefills the inherited definition's metadata when overriding, and `discover_secrets` still shows the broader definition's hint.

## Migration Plan

1. Ship schema v3 (`ALTER TABLE`, additive). Existing rows become `sensitive=1`, `kind=''`, `hint=''`, `value_plain=NULL`.
2. Export a passphrase backup before upgrading; it is the rollback artifact.
3. Old backups import unchanged (missing metadata defaults). New backups remain importable by old binaries for sensitive values; non-sensitive values are not.
4. Rollback: restore the pre-upgrade backup with the current or previous binary; there is no in-place down-migration.

## Open Questions

- Maximum hint length (a small bound prevents storage abuse; the exact number is a UI/validation detail and does not change the specs).
- Whether `list_secret_keys` should later include types (out of scope now; would be a separate change to the `mcp-server` spec).
