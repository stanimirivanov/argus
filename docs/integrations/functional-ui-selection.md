# Functional UI selection and catalog checking

## TL;DR

- `select -family functional-ui` emits a browser-test selection manifest, not
  an executable functional API plan.
- OpenAPI-only changes can target cataloged UI tests by explicit shared
  capabilities; component-root selection is a separate opt-in policy.
- Unknown, incomplete, or unmapped impact conservatively requires all cataloged UI tests.
- `playwright-catalog` checks a declared test inventory against a controlled
  Playwright `--list` result; it does not run tests or write the catalog.
- `plan-browser` expands reviewed repository/project/environment bindings into
  selected jobs and mandatory full-suite controls; explicit attempt bindings
  produce one complete browser shadow report with selection misses.
- `plan-browser` expands reviewed repository/project/environment bindings into
  selected jobs and mandatory full-suite controls; explicit attempt bindings
  produce one complete browser shadow report with selection misses.

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
identity locally. Locator repair remains an M07 follow-up.
[ADR-0025](../decisions/0025-select-browser-tests-only-for-fully-covered-api-changes.md),
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
The functional API runner cannot consume it. Browser CI scheduling and shadow
comparison remain the next milestone slice.

## Execute one browser project and retain evidence

Run each selected repository, project, and environment group separately from a
verified test checkout. The command and artifact directory come from reviewed
CI configuration; neither comes from the manifest. The example fixture lives
under `examples/functional-ui/test-repository` and uses the adapter in its
parent directory. From the Argus root, with `ARGUS_DATABASE_URL` set:

~~~sh
export ARGUS_BROWSER_TEST_DIR="$PWD/examples/functional-ui/test-repository"
export ARGUS_BROWSER_ARTIFACT_DIR="$PWD/.local/browser-artifacts"
export ARGUS_BROWSER_ENVIRONMENT=ci
go run ./cmd/run-browser \
  -manifest browser-selection.json -attempt-id ci-browser-123 \
  -test-repository-id tests-1 -test-revision "$(git rev-parse HEAD)" \
  -project chromium-desktop -environment ci \
  -- node examples/functional-ui/playwright-adapter.mjs > browser-attempt.json
go run ./cmd/execution-evidence browser-ingest -file browser-attempt.json
~~~

`run-browser` accepts only `argus.dev/browser-selection-manifest/v1`. It checks
the entire manifest against its schema, binds its SHA-256 to the attempt,
selects exact stable tests for the requested group, and validates the adapter's
complete result set. A failed test still emits an attempt document before the
runner exits nonzero; CI must retain and ingest that output. Empty groups,
duplicate or extra results, wrong project/environment, malformed output, and
missing or corrupt local artifacts fail closed. The reference adapter runs
only when the requested environment matches the CI-owned environment binding;
the test checkout's Playwright configuration owns the corresponding target URL.
It runs
actual Playwright tests, verifies the checkout revision, and registers a trace,
screenshot, and accessibility snapshot for every test. Each artifact reference
names its test and SHA-256.

`browser-ingest` stores immutable `argus.dev/browser-execution-attempt/v1`
evidence. An exact retry is idempotent; reusing the attempt ID with different
content conflicts. Artifact bytes are checked when the local adapter emits the
attempt. The stored file URI and digest are reference metadata; database
registration does not guarantee later file availability or retention. CI must
publish the artifact directory with the same access controls as test output.
`make ui-validate` runs a real Chromium campaign and cleans its temporary
artifact directory.

## Plan and compare every browser group

Keep the selection manifest byte-for-byte unchanged through planning and
execution. Its digest is the exact file SHA-256 used by `run-browser`. Prepare
reviewed `argus.dev/browser-execution-bindings/v1` configuration with one
group per intended test repository, Playwright project, and environment. Each
group supplies a stable `groupKey`, exact `testRepository`, immutable
`testRevision`, `adapter: "playwright"`, `project`, and `environment`. Commands,
secrets, runner labels, and artifact policy stay in CI configuration.

~~~sh
go run ./cmd/plan-browser -manifest browser-selection.json \
  -bindings browser-execution-bindings.json > browser-plan.json
~~~

The `argus.dev/browser-execution-plan/v1` job array is ordered by group key,
with `selected` before `full-suite`. Every nonempty reviewed group has a
full-suite job. A selected job appears only if its project has an early-stage
test. Missing project coverage, duplicate group coordinates, revision
mismatches, and empty groups fail planning. Fallback decisions without project
hints apply to every reviewed project for their test repository.

Run each job with `run-browser`, passing its stage, repository, revision,
adapter, project, and environment from the plan. CI must verify the test
checkout SHA, supply an adapter command from reviewed configuration, and retain
stdout even when `run-browser` exits nonzero: a valid failed attempt is still
evidence. Ingest every emitted attempt with `execution-evidence browser-ingest`.
Do not let an early selected failure cancel full-suite control jobs. Upload the
attempt documents and artifact directory on success or failure, with the same
access controls as other browser test evidence.

After all jobs have run, create an
`argus.dev/browser-plan-attempt-bindings/v1` document listing each `groupKey`,
its `selectedAttemptId` (null for a full-only group), and its explicit
`fullSuiteAttemptId`. Then compare the whole plan:

~~~sh
go run ./cmd/execution-evidence browser-plan-shadow-report \
  -plan browser-plan.json -attempt-bindings browser-attempt-bindings.json \
  > browser-shadow-report.json
~~~

The `argus.dev/browser-plan-shadow-report/v1` document reports test-duration
reduction and failure recall across all groups, including full-only groups.
It lists each full-suite failure the selected stage missed or did not
reproduce. Comparison refuses missing controls, duplicate attempt IDs,
cross-project/environment results, mismatched revisions or adapter versions,
and incomplete group bindings. No attempt is chosen by recency. Full-suite job
outcomes remain authoritative for CI and release decisions; the shadow report
measures selection only.

The checked-in
[`shadow-campaign.mjs`](../../examples/functional-ui/shadow-campaign.mjs)
is a runnable CI reference. It deliberately fails an omitted receipt test,
persists all three attempts, and asserts two reported misses. The PostgreSQL
integration job runs it and uploads the retained evidence directory even if
the campaign step fails. This synthetic failure is expected only in the
reference campaign; a product full-suite failure must still fail its CI gate.
Set `ARGUS_BROWSER_CAMPAIGN_PLAN_ONLY=1` to validate its manifest and job plan
without a local PostgreSQL server or Chromium run.
The reviewed binding and control decision is recorded in
[ADR-0035](../decisions/0035-plan-browser-controls-from-reviewed-project-bindings.md).
