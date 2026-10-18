# Functional UI selection and catalog checking

## TL;DR

- `select -family functional-ui` emits a browser-test selection manifest, not
  an executable functional API plan.
- OpenAPI-only changes can target cataloged UI tests by explicit shared
  capabilities; component-root selection is a separate opt-in policy.
- Unknown, incomplete, or unmapped impact conservatively requires all cataloged UI tests.
- `playwright-catalog` checks a declared test inventory against a controlled
  Playwright `--list` result; it does not run tests or write the catalog.

## Check a Playwright UI catalog

In the test repository's controlled CI checkout, collect the complete
inventory without running browser tests:

~~~sh
PLAYWRIGHT_JSON_OUTPUT_NAME=playwright-list.json \
  npx playwright test --list --reporter=json
~~~

Each test in the declared `functional-ui`/`playwright` suite needs a stable
annotation in its Playwright declaration. An optional owner annotation and
ordinary Playwright tags are included in the inventory:

~~~ts
test("checkout succeeds", {
  tag: "@smoke",
  annotation: [
    { type: "argus.test-key", description: "checkout-happy" },
    { type: "argus.owner", description: "web-team" },
  ],
}, async ({ page }) => { /* existing test body */ });
~~~

Run the checker from Argus with the descriptor and JSON list from the intended checkouts:

~~~sh
go run ./cmd/playwright-catalog \
  -source-revision 0123456789abcdef0123456789abcdef01234567 \
  -suite checkout-ui \
  ./descriptor.json ./playwright-list.json
~~~

It fails when a declared test is absent, a discovered key is undeclared, a
key appears twice in one project, or collection contains errors or executed
results. It prints stable keys, project variants, tags, and owners for review.
This is a local conformance check, not a catalog write or trusted selection
signal. Run the **unfiltered** `--list` command at a controlled, pinned test
revision and supply the descriptor from its claimed source revision; Argus
does not verify either checkout here. Playwright collection evaluates
repository code, so it must run only in appropriately isolated CI.

## Select cataloged UI tests

After the approved base descriptor and change impact have been stored, select by explicit family:

~~~sh
go run ./cmd/select \
  -provider github \
  -family functional-ui \
  -delivery-id 01234567-89ab-cdef-0123-456789abcdef \
  > browser-selection.json
~~~

This emits `argus.dev/execution-manifest/v2`, separate from the default
functional API v1. Only an OpenAPI-only change with complete mapped impact
can target UI tests by shared capability. Any other changed file, truncated
file list, incomplete impact, or unmapped operation requires every cataloged
UI candidate. Omitted early-stage tests retain a full-suite obligation.

For source changes covered by the approved base descriptor's declared
component roots, opt into a different policy:

~~~sh
go run ./cmd/select \
  -family functional-ui \
  -ui-impact components \
  -delivery-id 01234567-89ab-cdef-0123-456789abcdef \
  > browser-component-selection.json
~~~

This emits `argus.dev/execution-manifest/v3`. Every changed file and
rename/copy predecessor must match a declared component root. A missing root,
empty or truncated file list, or invalid catalog evidence prevents targeted
omissions. Roots match complete path segments, and overlapping mappings are
combined. The v3 policy does not parse routes or run Playwright.

The functional API planner and runner accept only v1; they **must not**
execute these browser manifests. The Playwright checker validates test
identity locally. Browser execution evidence and locator repair remain M07
follow-ups. [ADR-0025](../decisions/0025-select-browser-tests-only-for-fully-covered-api-changes.md),
[ADR-0026](../decisions/0026-check-playwright-inventory-against-declared-tests.md),
and [ADR-0027](../decisions/0027-opt-in-browser-selection-from-declared-component-roots.md)
define the policy boundaries.

## Persist reviewed discovery and select from source mappings

In a controlled CI checkout, the checker emits a schema-compatible suite
observation instead of its human summary:

~~~sh
go run ./cmd/playwright-catalog \
  -source-revision 0123456789abcdef0123456789abcdef01234567 \
  -suite checkout-ui -suite-json \
  -test-revision fedcba9876543210fedcba9876543210fedcba98 \
  ./descriptor.json ./playwright-list.json > checkout-suite.json
~~~

CI must check the actual source and test checkout SHAs before invoking this
command. Its revision flags are assertions, not proof of filesystem state.
Collect every declared Playwright UI suite at an unfiltered test checkout.
Compose their suite JSON objects into one `argus.dev/browser-discovery/v1`
document with a stable `observationId`, catalog snapshot reference, collector
version, collection and expiry timestamps, and reviewed mappings. Set
`complete` only after every suite collection succeeds; set `mappingComplete`
only when every declared test has a reviewed route or component mapping.
`mappingRevision` names the source revision where the mapping was reviewed and
must match the catalog revision for targeted selection. The
[browser discovery schema](../../contracts/generated/browser-discovery/v1/browser-discovery.schema.json)
defines the document shape.

Each mapping names a stable test key, source root, and route or component key.
A route also records its reviewed URL pattern. Component mappings must use a
component key and root from the descriptor. Playwright titles, tags, and owners
never become selection policy. A route key cannot claim conflicting roots or
patterns. Incomplete observations may be stored for diagnosis but cannot omit
tests.

After the source catalog snapshot is ingested, persist and inspect the
observation with `ARGUS_DATABASE_URL` set:

~~~sh
go run ./cmd/browser-catalog ingest -file browser-discovery.json
go run ./cmd/browser-catalog get -observation-id ci-run-123
~~~

The observation ID is immutable: an exact retry creates no second row and
changed content conflicts. CI pins each intended test checkout in a reviewed
`argus.dev/browser-suite-bindings/v1` document. Select from a verified delivery
and explicit evaluation instant:

~~~sh
go run ./cmd/select -family functional-ui -ui-impact mappings \
  -delivery-id 01234567-89ab-cdef-0123-456789abcdef \
  -ui-observation-id ci-run-123 \
  -ui-suite-bindings-file browser-suite-bindings.json \
  -evaluated-at 2026-10-18T07:00:00Z > browser-selection.json
~~~

This emits `argus.dev/browser-selection-manifest/v1`. Every changed path,
including a rename or copy predecessor, must fall under a reviewed root.
Missing, expired, incomplete, or mismatched discovery, incomplete changed-file
evidence, and unmapped paths require all declared UI tests. A targeted omission
still carries a full-suite execution obligation. Targeted decisions keep
project variants; fallback decisions clear project hints and require the full
suite. The manifest records the changed paths and discovery digest for review.
The browser executor and CI
scheduler are subsequent PRs; the functional API runner cannot consume it.
