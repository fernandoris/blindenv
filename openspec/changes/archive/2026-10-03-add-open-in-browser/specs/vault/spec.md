## ADDED Requirements

### Requirement: Project browser-open capability

The vault SHALL store a per-project boolean capability `allow_open` that controls whether the MCP `open_in_browser` Tool may open a URL for that project. A newly created project SHALL default `allow_open` to disabled. The capability SHALL be readable and writable independently of `allow_execute`. Existing vaults SHALL be migrated so that every existing project has `allow_open` disabled.

#### Scenario: New project defaults disabled

- **WHEN** a project is created without specifying `allow_open`
- **THEN** its `allow_open` is disabled

#### Scenario: Capability is independent of execution

- **WHEN** `allow_execute` is enabled for a project and `allow_open` is not changed
- **THEN** `allow_open` remains in its previous state

#### Scenario: Existing projects migrate disabled

- **WHEN** a vault created before this capability existed is opened by a newer version
- **THEN** every existing project has `allow_open` disabled
