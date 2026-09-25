## Why

BlindEnv models secrets at only two levels, both scoped to a single project: project-global and project+environment. There is no way to define a value once for an environment across every project, nor one for the whole vault, so the same key and value get duplicated across projects. That drift makes rotation error-prone and widens the leak surface. Adding the two cross-project tiers is the product ask; the current dashboard, which stacks one equal-weight block per scope, cannot present four tiers without collapsing into an unreadable wall.

## What Changes

- Add two shared scopes: **environment-global** (one environment name, all projects) and **global** (all projects, all environments). Secrets stay defined in exactly one of four scopes.
- Define a single precedence ladder, most specific wins: `project+environment > project-global > environment-global > global`. Project beats environment on the tie because an explicit project setting must be able to shadow a shared default.
- **BREAKING** (storage schema v2): unify secrets around a scope address (`project_id` and `environment` nullable, four partial unique indexes). Project+environment secrets move from an `environment_id` foreign key to the environment **name**, so referential cascade must be handled explicitly. Existing vaults migrate in place.
- Backup export/import carries the shared scopes; the vault schema version is bumped.
- MCP resolution (`list_secret_keys`, `get_context`, `proxy_http_request`, `execute_with_secrets`) draws from the effective four-tier set, and audit entries record the winning scope for each resolved key.
- Redesign the dashboard: master-detail scope rail with a **Shared** section plus projects, one scope editor at a time, defined vs inherited vs effective resolution separated, audit moved to a slide-over, blast-radius confirmation before deleting shared secrets, inline toasts, reveal auto-hide, and a real accessibility baseline.
- Update documentation: README (scope model, precedence, dashboard, CLI/env), and CONTRIBUTING where the artifact workflow is described.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `vault`: the secret scope model gains environment-global and global shared scopes with a four-tier precedence, and the uniqueness/resolution requirements change accordingly.
- `mcp-server`: tool resolution covers the effective four-tier set instead of only the project's global and environment keys, and the audit entry records the source scope.
- `web-ui`: navigation and management move to a master-detail model with a Shared section, a definition-vs-resolution split, an audit slide-over, and safeguards for high-blast-radius actions.

## Impact

- **Storage / migration** (`pkg/db/schema.go`, `pkg/db/store.go`): new scope-addressable `secrets` shape, migration from `schemaVersion` 1 to 2, `Resolve` / `ListKeys` / `ListSecrets` / `ScopeValues` / `PutSecret` / `DeleteSecret` become scope-aware.
- **Domain** (`pkg/db/models.go`): `Scope` grows to four values; `SecretInfo` gains source/provenance information.
- **Backup** (`pkg/backup`): export/import must include shared scopes and the new schema.
- **MCP** (`pkg/mcp`): resolution and audit changes only; tool signatures unchanged.
- **Web API** (`pkg/web/server.go`): state payload gains a `shared` section and provenance; reveal must be scope-qualified.
- **Web UI** (`web/`): structural rewrite of the dashboard `app.js` and layout in `index.html`; Tailwind classes only, no new JS dependencies.
- **Docs** (`README.md`, `CONTRIBUTING.md`): scope model, precedence, dashboard and workflow sections updated.
- **Risk**: a global or environment-global secret is injected into every project that resolves it, so `execute_with_secrets` in any project can expose it. This change increases the blast radius of a single secret and must be paired with the delete-impact confirmation and audit provenance above.
