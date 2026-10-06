## Why

On `ubuntu-latest` the `pkg/mcp` test suite has failed since commit `d7bc6ed`:
`TestOpenResolvesSensitiveTagWithoutLeak`, `TestOpenNonMatchingTagPassesThrough`
and `TestOpenReportsSubstitutionsAndUnmatched` expect a successful
`open_in_browser` call, but `handleOpen` refuses on a headless Linux session
before it reaches `browserLauncher`, so the display-less CI runner returns
"no graphical display detected". The product behavior is correct - only the
tests are wrong. The failure was masked by the formatting gate that failed
first on every push from `e0ea30f` onward.

## What Changes

- Make the graphical-display guard overridable by tests, mirroring the
  existing `browserLauncher` seam, so the substitution/audit/launch path can be
  exercised without a real display.
- Update the three affected tests to opt into "display available"; keep
  `TestOpenHeadlessLinuxRefused` (asserts refusal) and `TestHasDisplay`
  untouched.
- No product behavior change: a headless Linux session still refuses to open a
  URL.
- **Out of scope**: the `windows-latest` `TestSnapshotAllocationsBounded`
  failure (tracked separately).

## Capabilities

### New Capabilities
<!-- None: this is a test-only fix, no spec-level behavior change.
     The change opts out of specs via skip_specs: true. -->

None.

### Modified Capabilities

None.

## Impact

- `pkg/mcp/open.go` - test seam for the display guard (no behavior change).
- `pkg/mcp/open_test.go` - the three affected tests.
- `.openspec.yaml` - sets `skip_specs: true`.
