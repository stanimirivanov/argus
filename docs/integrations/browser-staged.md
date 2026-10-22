# Opt-in browser staged execution

## TL;DR

- M08-PR11 executes one portfolio episode using an independent browser manifest,
  complete discovery, and reviewed project/environment bindings.
- Apply the forward migration and deploy the v2 runner, adapter and evidence
  reader before explicitly opting in with `-schedule`.
- Shadow requires relevant-now and a full shadow-control. Staged requires
  relevant-now and its exact remaining-later complement, even after early failures.
- V1 selected/full-suite behavior stays compatible. Individual stages grant no
  release or promotion authority; aggregate monitoring belongs to M08-PR12.

## Trust and complete inventory

Use unchanged output from the [portfolio command](predictive-portfolio.md).
`-schedule-sha256` is a trusted SHA-256 of the complete file bytes; `-episode`
selects one exact episode. Inputs remain retrospective/offline evidence, not a
new live prediction. Trusted CI authenticates and retains these files and the
reviewed configuration, and provisions immutable test checkouts and credentials.

Both planner and runner validate the independent browser-selection-manifest/v1,
browser-discovery/v1 and browser-execution-bindings/v1. Existing bindings already
pin repository, revision, adapter, project and environment and need no new version.
Every discovered project variant must be covered exactly, with matching titles
and revisions; repeated environments may cover the same project variant. Duplicate,
missing or extra variants/groups and incomplete discovery fail before dispatch.
Targeted manifests additionally require complete mappings and exact canonical
catalog evidence digest, observation ID, mapping revision and collector version.
A separate raw discovery-file SHA-256 is retained in v2 plans and attempts.
Discovery expiry is selection-time evidence, not a new wall-clock policy at replay.

Candidate IDs retain the archived repository/family/adapter/project/environment
coordinates. Reviewed cost environments and dependencies must match the complete
group inventory. The runner revalidates the whole plan against independently
loaded source inputs and the required CI mode, then checks explicit checkout,
project and environment arguments before opening the adapter.

## Plan and run

From an Argus checkout with the commands built or available through Go:

~~~sh
go run ./cmd/plan-browser \
  -manifest manifest.json -bindings bindings.json -discovery discovery.json \
  -schedule portfolio.json -schedule-sha256 "$PORTFOLIO_SHA256" \
  -episode episode -mode shadow > plan-v2.json

go run ./cmd/run-browser \
  -manifest manifest.json -bindings bindings.json -discovery discovery.json \
  -plan plan-v2.json -schedule portfolio.json \
  -schedule-sha256 "$PORTFOLIO_SHA256" -episode episode -mode shadow \
  -group desktop -project chromium-desktop -environment ci \
  -stage relevant-now -attempt-id github-run-early-1 \
  -test-repository-id tests-1 -test-revision aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  -- node tools/argus-playwright-adapter.mjs > attempt-v2.json
~~~

The adapter path is the reviewed test checkout's installed copy of
`examples/functional-ui/playwright-adapter.mjs`; retain its existing environment
and artifact-root configuration from the [UI guide](functional-ui-selection.md).
Only the manifest may use stdin on the runner; other staged inputs must be files.
Mode defaults to shadow on opt-in. Stage must be explicit; the v1 default selected
is rejected with a schedule. A v2 adapter request includes both the complete
candidate inventory and the ordered tests requested for this phase. The reference
adapter runs fresh, unfiltered project-specific Playwright `--list` and checks
exact inventory before executing any requested test. Older strict adapters must
be upgraded before opt-in.

For each group, use a distinct attempt ID to run shadow-control after relevant-now
in shadow mode. Alternatively plan and run with mode staged and execute
remaining-later after relevant-now. CI must run these obligatory passes using
failure-independent conditions; persist stdout even when tests fail. An empty
phase emits a skipped receipt with group/stage, opens no adapter and creates no
synthetic evidence. Retain its plan as membership evidence.

Early execution preserves portfolio order. Later and control passes use stable
topological order; the reference adapter executes serially. Cross-group
dependencies fail because this CLI cannot coordinate arbitrary interleaving.
CI must serialize group jobs sharing reviewed resource capacity: separate workers
cannot be governed by one command. The runner does not provision environments,
choose adapter commands from artifacts or authorize a release.

## Evidence and recovery

Browser adapter requests, plans and execution attempts have explicit v2 versions.
Browser adapter results remain v1, preserving exact result membership, outcome,
failure and artifact semantics. Traces, screenshots and accessibility artifacts
are still required for every non-skipped result, and their actual file bytes are
verified before returning an attempt. Missing/substituted results or artifacts,
invalid JSON, process failures, timeout and cancellation cannot become success.
Failed, skipped or errored tests produce valid failed/incomplete/error evidence
on stdout followed by a nonzero status.

Ingest explicit v1 or v2 evidence with the existing command:

~~~sh
go run ./cmd/execution-evidence browser-ingest -file attempt-v2.json
~~~

Exact retries are idempotent; changing membership, results or any provenance
under the same attempt ID conflicts. Genuine reruns need new IDs. Ingestion
validates normalized evidence shape; genuine runner origin remains a trusted CI
responsibility. Legacy-only shadow and locator consumers deliberately reject v2.
Full staged safety aggregation and promotion remain M08-PR12 and M08-PR13.

## Deployment and verification

Apply `20261022062238_add_predictive_browser_stages.sql` using the existing
migration workflow before enabling v2. It adds nullable schedule/episode/group,
mode, discovery digest and stage columns to browser attempt headers, with a
version-specific envelope constraint. Old rows keep NULL values and their exact
retry fingerprints. No backfill, row rewrite or new index is required. Constraint
validation scans the header table and ALTER TABLE takes locks: choose a deployment
window appropriate to the table size. Payload checksums and header provenance
are checked together on read.

Deploy updated readers, planner, runner and adapter before opt-in. Old writers
remain compatible, but old readers cannot interpret v2 rows. Behavior rollback
disables schedule flags and restores selected/full-suite jobs while retaining the
forward schema and immutable evidence. Failed transactional migrations roll back
and can be retried; never edit applied migrations or rewrite attempts. Existing
evidence permissions and retention still apply.

Run `make generate-contracts`, `make ui-validate`, `make db-validate` and
`make validate`. Regression tests cover real Chromium stage membership and fresh
inventory, project/environment isolation, incomplete discovery, exact partitions,
artifact corruption, cancellation, empty phases, strict v1 rejection, released
schema upgrades, immutable concurrent retries and conflicts. See
[ADR-0049](../decisions/0049-opt-in-browser-staged-execution.md) and the
[shared staged invariants](staged-execution.md).
