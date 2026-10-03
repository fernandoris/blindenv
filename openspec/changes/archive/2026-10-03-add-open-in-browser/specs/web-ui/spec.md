## ADDED Requirements

### Requirement: Browser-open permission control

The UI SHALL allow enabling or disabling opening URLs in the browser (`allow_open`) per project, independently of execution with secrets (`allow_execute`), and SHALL make clear that enabling it lets an agent hand a resolved URL to the local default browser.

#### Scenario: Permission change

- **WHEN** the user disables `allow_open` for a project
- **THEN** invocations of `open_in_browser` on that project are rejected

#### Scenario: Independent from execution

- **WHEN** the user toggles `allow_open`
- **THEN** the `allow_execute` state is unchanged
