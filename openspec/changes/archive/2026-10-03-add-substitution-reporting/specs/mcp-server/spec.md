## ADDED Requirements

### Requirement: Substitution and unmatched-tag reporting

The secret-consuming Tools SHALL report, in their successful result payload, a request-side substitution count and the names of any `{{...}}` tags that did not name an effective key. The substitution count SHALL be the number of `{{SECRET_NAME}}` occurrences replaced (or, for `execute_with_secrets`, rewritten to the target shell's native environment reference) across the request the Tool sends: URL, header values and body for `proxy_http_request`; command and arguments for `execute_with_secrets`; URL for `open_in_browser`. A repeated tag SHALL count once per occurrence. A tag naming a non-sensitive effective key SHALL count as a substitution, consistent with it still being substituted. `unmatched_tags` SHALL report each distinct unmatched tag name at most once, SHALL NOT contain a tag name outside the input the caller supplied, and SHALL NOT contain any secret value. The reported list SHALL be bounded so it cannot grow with the request size. `redactions` SHALL remain a response-side count and MUST NOT be conflated with the substitution count.

#### Scenario: Proxy reports substituted occurrences

- **WHEN** `proxy_http_request` replaces a `{{SECRET_NAME}}` tag that appears in a header and again in the body
- **THEN** the result reports a substitution count of two

#### Scenario: Execute reports translated references

- **WHEN** `execute_with_secrets` rewrites matching `{{SECRET_NAME}}` tags in a command to the shell's native environment reference
- **THEN** the result reports a substitution count equal to the number of matching tags rewritten

#### Scenario: Open reports URL substitutions

- **WHEN** `open_in_browser` resolves a URL containing one `{{SECRET_NAME}}` tag that names an effective key
- **THEN** the result reports a substitution count of one

#### Scenario: No tag used

- **WHEN** a request contains no `{{...}}` tag
- **THEN** the result reports a substitution count of zero and an empty unmatched-tag list

#### Scenario: Unmatched tag reported by name

- **WHEN** a request contains `{{MISSING_KEY}}` and no effective key is named `MISSING_KEY`
- **THEN** the result reports zero substitutions and lists `MISSING_KEY` among the unmatched tags

#### Scenario: Repeated unmatched tag listed once

- **WHEN** the same unmatched tag name appears several times in one request
- **THEN** the result lists that name exactly once

#### Scenario: Substitution without response redaction

- **WHEN** a tag naming a sensitive effective key is substituted into the request and the response does not contain the value
- **THEN** the result reports the substitution count greater than zero and a redaction count of zero

#### Scenario: Unmatched tag list is bounded

- **WHEN** a request contains more distinct unmatched tag names than the reporting maximum
- **THEN** the result reports at most the fixed maximum distinct names without error

#### Scenario: Non-sensitive tag counts as a substitution

- **WHEN** a request substitutes `{{ENDPOINT}}` and `ENDPOINT` is a non-sensitive effective key
- **THEN** the result reports it as a substitution

### Requirement: Audit records the substitution count

The audit entry for a secret-consuming Tool invocation SHALL record the request-side substitution count alongside the existing redaction count. The audit entry MUST NOT contain secret values. Unmatched tag names SHALL NOT be persisted in the audit entry.

#### Scenario: Substitution count persisted

- **WHEN** `proxy_http_request` or `execute_with_secrets` substitutes one or more tags
- **THEN** the audit entry records the matching substitution count

#### Scenario: Refused invocation records zero

- **WHEN** a secret-consuming Tool refuses a call before substituting any tag
- **THEN** the audit entry records a substitution count of zero
