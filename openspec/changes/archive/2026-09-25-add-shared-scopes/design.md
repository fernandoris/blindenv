## Context

See `proposal.md - Why`. Today `secrets` carries `project_id NOT NULL` and a nullable `environment_id` FK (`pkg/db/schema.go:31-45`), so a secret is always owned by exactly one project and scope is inferred from `environment_id IS NULL`. Resolution (`pkg/db/store.go:320-344`, `:441-459`) merges a project's globals with one environment. `SecretInfo` exposes `Scope` with two values and an `Overrides` bool (`pkg/db/models.go:5-38`). The dashboard renders one block per project scope (`web/app.js:139-140`), which already conflates owned and inherited rows.

Two levels must become four, two of them cross-project, and the dashboard must stop scaling by number of scopes.

## Goals / Non-Goals

**Goals:**

- A single scope-addressable storage shape that makes the four tiers first-class and keeps resolution a single ordered query.
- A precedence contract that is unambiguous and lets an explicit project setting shadow a shared default.
- Provenance available to the UI and the audit log without ever carrying values in state payloads.
- A dashboard that stays readable at any number of projects/environments and makes inheritance obvious.
- In-place migration of existing vaults and backward-compatible backup import.

**Non-Goals:**

- No global environment registry: environment sharing is by name, not by a new canonical table.
- No environment rename support; no per-secret access control; no secret versioning.
- No production crown jewels: the existing threat model and "no subcommand prints values" guarantee are unchanged.
- No new JS framework or build step; the dashboard stays vanilla JS + Tailwind.

## Decisions

### D1. Unified scope-addressable `secrets` table

Replace the `environment_id` FK with a nullable `project_id` plus a nullable `environment` **name**. Four combinations are the four scopes:

| `project_id` | `environment` | Scope |
| --- | --- | --- |
| not null | not null | project + environment |
| not null | null | project-global |
| null | not null | environment-global (shared) |
| null | null | global (shared) |

Uniqueness is enforced with four partial unique indexes (mirroring the two that exist today), so SQLite's NULL-distinctness does not matter.

- **Why over separate tables**: resolution is one query with a `CASE` priority and a "highest wins" merge; backup enumerates one table; list/merge code is not duplicated per tier.
- **Why over keeping `environment_id` for project rows**: two representations of the same environment at different scopes invite drift and complicate provenance. Dropping the FK costs only the implicit delete cascade, which the store already handles explicitly.
- **Trade-off accepted**: environment identity is now a string; "Dev" and "dev" are different. Mitigated in D3.

### D2. Precedence ladder and tie-break

Priority `4 > 3 > 2 > 1`: project+environment > project-global > environment-global > global. Both middle tiers bind one axis, so the tie is broken in favor of the **project** axis: an explicit project setting must be able to shadow a shared environment default. This mirrors CSS specificity (project ~ id, environment ~ class).

Resolution query returns all rows matching `(project_id = :p OR :p IS NULL)` and `(environment = :e OR :e IS NULL)` ordered by ascending priority; the store overwrites as it iterates, so the last (most specific) wins.

- **Alternative rejected**: environment-global over project-global. It would let a shared default silently beat a project's own value, which is surprising and hard to debug.

### D3. Environment sharing by name, no registry

The set of shared environments offered by the UI is derived: the union of every project's environment names plus any name already carrying an environment-global secret. Environment names are matched exactly.

- **Why**: a canonical registry is a second large refactor (project↔environment association), and the feature does not require it. The UI can still suggest existing names, which prevents most drift.
- **Risk**: naming drift (`Dev` vs `dev`) silently fails to match. Mitigation: the shared scope editor shows the literal name and the derived set; a future global registry is the clean fix and is out of scope here.

### D4. Provenance computed server-side, values never leave the store

`SecretInfo` gains a four-value `Scope` and the "shadowed scopes" it overrides. A new read-only effective view returns `key -> winning scope` for a `(project, environment)`. The reveal endpoint is scope-qualified so the UI can reveal the exact definition. No state payload ever includes values.

- **Why server-side**: the precedence rule lives in one place, the audit log can record the source scope, and the client does not re-implement resolution.

### D5. Audit records the source scope

`audit_log` gains a `key_scopes` column pairing each used key with the scope it resolved from. It still stores names only. This makes "where did this value come from?" answerable, which is essential once a shared secret can appear in any project.

### D6. Schema migration v1 -> v2, in place

Inside a transaction: create the new `secrets` shape, copy rows joining `environments` to resolve names (`LEFT JOIN` so project-global rows get `NULL`), drop the old table, rename, create the four partial unique indexes, add `audit_log.key_scopes`, write `schema_version = 2`. `environments` is untouched.

`DeleteEnvironment` and `DeleteSecret` become explicit `WHERE project_id = ? AND environment = ?` deletes. Backup export writes v2; import accepts v1 by mapping the old project/environment ids to names and v2 unchanged.

### D7. Dashboard: master-detail, definition vs resolution

One scope editor at a time, chosen from a rail with a **Shared** section (global, then each shared environment) and a **Projects** section (collapsible project groups, then project-default and each environment). The editor separates **Defined here** (editable) from **Inherited** (ghost rows with a source badge and an `override` action) and offers a read-only **effective preview** listing the winning scope per key. The audit log moves to a header-triggered slide-over. Shared deletions require typed confirmation showing which scopes lose the key. Reveals auto-hide and are scope-qualified.

- **Why**: it is the only structure that does not grow with project x environment count, and it separates the two questions the current single table answers at once.
- **Sequencing**: the rail/master-detail and the audit drawer work against the current model too, so the UI shell lands first and the Shared tiers slot in as an additive rail section.

### D8. Documentation updated with the change

`README.md` (scope model and precedence, dashboard description, CLI/environment notes) and `CONTRIBUTING.md` (the artifact flow, if it references scopes) are updated in the same change so the feature ships documented.

## Risks / Trade-offs

- **Blast radius of a shared secret** -> it resolves in every project, so any project's `execute_with_secrets` can expose it. Mitigation: typed-confirmation delete impact, audit source scope, inline warning in the shared scope editor, and unchanged redaction/taboo on value-printing.
- **Migration irreversibility** -> the table is recreated. Mitigation: transactional migration, and users are told to export a backup first; import of v1 backups stays supported.
- **String environment matching drift** -> see D3; documented, UI suggests names.
- **Loss of at-a-glance overview** -> master-detail hides other scopes. Mitigation: rail counts, effective preview, rail filter.
- **Reveal is currently unaudited** -> a shared reveal could be silent. Mitigation: scope-qualified reveal with auto-hide; consider auditing reveals as part of D5.
- **Accessibility debt** -> drawer/dialog/focus management in vanilla JS. Mitigation: explicit a11y tasks (aria-current/expanded/pressed, `<th scope>`, focus trap, `aria-live`), and never encoding tier by color alone.

## Migration Plan

1. Ship the storage + API changes behind the migration; existing 2-tier vaults upgrade on open.
2. Back up before upgrading; export v2, import accepts v1 and v2.
3. Rollback: keep the pre-upgrade backup; a v2 vault is not readable by older binaries, so downgrade is restore-from-backup.

## Open Questions

- Whether to replace name-derived sharing with an explicit global environment registry later (deferrable; does not change specs or tasks).
- Final UI copy for tier labels (deferrable; wording only).
