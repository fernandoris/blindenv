## Context

See `proposal.md - Why`. Today the request and response pipelines are asymmetric in observability:

```
  REQUEST SIDE                        RESPONSE SIDE
  substitute()/translateSecretTags()  Redactor.Redact()
  result count discarded              result count = redactions (reported + audited)
```

`substitute` (`pkg/mcp/proxy.go:130`) returns only the resolved string. `translateSecretTags` (`pkg/mcp/shell.go:89`) returns only `(string, error)`. Neither surfaces how many tags were replaced, and no code collects tag names that failed to match an effective key. The audit table (`pkg/db/schema.go:43`) has only `redactions`. Migrations are additive and guarded by `columnExists` (precedent: `key_scopes` added in `pkg/db/schema.go:112`). Backups (`pkg/backup/backup.go`) cover projects, environments and secrets, not the audit log.

## Goals / Non-Goals

**Goals:**
- Report request-side substitution counts and unmatched tag names for all three secret-consuming Tools.
- Persist the substitution count in the audit log.
- Keep peak memory and allocation growth flat relative to the already-capped request sizes.

**Non-Goals:**
- Changing the response-side `redactions` semantics, format, or column.
- Persisting `unmatched_tags` in the audit log.
- Reporting substitution counts for discovery/listing Tools (`list_secret_keys`, `discover_secrets`, `get_context`), which do not consume secrets.
- Redacting or hiding the unmatched tag names, which are caller-supplied text, never values.

## Decisions

### Decision: Count occurrences, not distinct keys

`substitutions` is the number of `{{SECRET_NAME}}` occurrences replaced. This matches the operator's mental model ("how many placeholders went into the request") and mirrors `redactions`, which already counts occurrences (`strings.Count` in `Redactor.Redact`, `pkg/mcp/redact.go:54`).

*Alternative considered:* distinct keys substituted. Rejected: it would mask a body that substitutes the same key many times and would read inconsistently next to `redactions`.

### Decision: Report `unmatched_tags` as deduplicated names, capped

A tag whose name is not an effective key produces `substitutions` contribution of zero; reporting the name converts an ambiguous `0` into a precise diagnosis (for example `{{RANCHER_TOKEN}}` versus a key named `RANCHER_API_TOKEN`). The list is deduplicated and capped at **20** distinct names so a pathological body cannot grow the response. Names are caller-supplied identifiers, so no secret value is exposed.

*Alternative considered:* a boolean `had_unmatched_tag`. Rejected: it loses which tag, which is the actionable part.

### Decision: Add `substitutions`; leave `redactions` untouched

Keep `redactions` as the response-side metric and add `substitutions` next to it. Result shapes become:

```
  proxy_http_request   -> { status, headers, body, substitutions, unmatched_tags, redactions }
  execute_with_secrets -> { exit_code, stdout, stderr, substitutions, unmatched_tags, redactions }
  open_in_browser      -> { opened, substitutions, unmatched_tags }
```

`open_in_browser` has no `redactions` field because it returns no response body; adding one would imply it scrubs something.

*Alternative considered:* rename `redactions` to `redactions_made` or `secrets_scrubbed` for symmetry. Rejected: it changes a persisted column and an existing response field for cosmetic gain; the docs and the paired `substitutions` field resolve the ambiguity.

### Decision: Persist the substitution count in the audit log

Add `substitutions INTEGER NOT NULL DEFAULT 0` to `audit_log`, bump `schemaVersion` from 4 to 5, and add a guarded additive migration mirroring `migrateAllowOpen`. The audit is the durable forensic record; the 401 investigation happens there, not in the model's ephemeral context. `unmatched_tags` is intentionally not persisted.

*Alternative considered:* response-only. Rejected: it leaves the audit exactly as ambiguous as before.

### Decision: Fold counting into the substitution helpers

- `substitute` returns `(string, int)`; `substituteBody` returns `(string, int)`. Counting uses `strings.Count` before each `strings.ReplaceAll`, or a single counting replace, whichever benchmark favors.
- `translateSecretTags` returns the rewritten string, the rewritten-tag count, and the unmatched tag names; the existing single character scan already visits every tag, so counting folds in without a second pass.
- A shared helper collects distinct unmatched names (bounded at 20) by scanning for `{{`/`}}` in the caller-supplied text and excluding effective keys.

### Decision: Clarify terminology in `StreamRedactor`

`pkg/mcp/capture.go:84` documents `Count()` as "number of substitutions performed so far" while it counts redactions. Rename the wording to redactions to avoid colliding with the new request-side meaning. No behavior change.

## Risks / Trade-offs

- [Extra scan per key in `substitute`] -> `strings.Count` is a cheap index scan; measure with `BenchmarkSubstituteCount` and prefer a single counting-replace if the double scan shows up. Request sizes are already capped (proxy body `1<<20`, `pkg/mcp/proxy.go:20`; exec capture `maxOutputBytes + MaxValueLen`, `pkg/mcp/exec.go:74`).
- [Semantic mismatch: exec "substitutes" by translating to a native reference, not by placing the value] -> Document in the field semantics and in the spec scenario "Execute reports translated references"; the count still answers "did the tags the caller wrote reach the child".
- [`unmatched_tags` noise when a caller legitimately wants a literal `{{...}}`] -> Capped, deduplicated, and advisory only; it never changes execution.
- [Additive migration on an existing vault] -> Guarded `ALTER TABLE ... ADD COLUMN` with a safe default; the audit table is not part of backups, so no backup-format coupling. Rollback leaves an unused column, which is harmless.

## Migration Plan

1. Bump `schemaVersion` to 5.
2. Add `migrateAuditSubstitutions` guarded by `columnExists(ctx, "audit_log", "substitutions")`; on upgrade every existing row defaults to 0.
3. Extend `AuditEntry`, the insert and the select in `pkg/db/store.go`, and the audit view in `pkg/web/server.go`.
4. Rollback: revert the code; the extra column is ignored by the previous build.

## Open Questions

None.
