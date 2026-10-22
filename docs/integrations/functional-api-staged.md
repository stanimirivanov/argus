# Opt-in functional API staged execution

## TL;DR

- M08-PR10 consumes a complete predictive portfolio episode and independent execution manifest plus reviewed v2 bindings.
- Apply the forward migration, deploy the new planner, runner and evidence reader together, then opt in explicitly with `-schedule` and its trusted full-file SHA-256.
- Shadow runs relevant-now plus an independent full shadow-control; staged runs relevant-now plus the exact remaining-later complement.
- CI must execute required later/control work even if the early job fails. An early subset or a successful individual attempt grants no release/promotion authority.
- Existing invocations, v1 wire validators and selected/full-suite behavior remain unchanged.

## Inputs and trust

Use the [portfolio command](predictive-portfolio.md) output unchanged. `-schedule-sha256` is the trusted SHA-256 of the **complete file bytes**, not a digest copied from an untrusted plan. `-episode` names one exact episode. The planner and runner independently check the raw artifact schema, full partition, candidate identities, manifest digest and reviewed environment assignments. Unknown fields and inconsistent evidence are rejected. Portfolio evidence is not authenticated approval: trusted CI must authenticate and retain the inputs and reviewed configuration.

The reviewed bindings version is `argus.dev/functional-api-execution-bindings/v2`:

~~~json
{
  "apiVersion": "argus.dev/functional-api-execution-bindings/v2",
  "groups": [{
    "groupKey": "orders",
    "testRepository": {"provider": "github", "host": "github.com", "providerRepositoryId": "tests-1", "owner": "example", "name": "orders-tests"},
    "testRevision": {"algorithm": "git-sha1", "digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
    "adapter": "playwright",
    "environment": "ci"
  }]
}
~~~

Bindings must cover every complete manifest repository/adapter group exactly. Revisions and environments come from reviewed CI configuration, never model output. Archived API test IDs retain null project/environment coordinates; the cost review and v2 binding independently pin the actual execution environment. No fabricated history or implicit latest lookup is used. Current portfolios remain retrospective/offline artifacts; this runner executes their exact named episode and does not generate a new live prediction.

## Plan and run

~~~sh
go run ./cmd/plan-functional-api \
  -manifest manifest.json -bindings bindings-v2.json \
  -schedule portfolio.json -schedule-sha256 "$PORTFOLIO_SHA256" \
  -episode episode -mode shadow > plan-v2.json

go run ./cmd/run-functional-api \
  -manifest manifest.json -bindings bindings-v2.json -plan plan-v2.json \
  -schedule portfolio.json -schedule-sha256 "$PORTFOLIO_SHA256" \
  -episode episode -mode shadow -group orders -environment ci \
  -stage relevant-now -attempt-id github-run-early-1 \
  -test-repository-id tests-1 -test-revision aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  -adapter playwright -- node tools/argus-playwright-adapter.mjs execute \
  > attempt-v2.json
~~~

The adapter path is the test checkout's reviewed installation of the reference adapter, as in the [reference integration](reference-playwright-api.md). On the runner, auxiliary staged inputs are files and only the manifest may use stdin. The planner retains its existing manifest-or-bindings stdin option. `-mode` defaults to `shadow` when opted in, and must match the independently required CI mode. `-stage` is explicit; legacy default `selected` is rejected for v2. The runner validates the whole plan and checkout identity/revision/environment before creating the adapter process.

For each group, run `shadow-control` after `relevant-now` in shadow mode, with a distinct attempt ID. For staged mode, generate a plan with `-mode staged` and run `remaining-later` after `relevant-now`. All required passes must be scheduled with failure-independent CI conditions. The plan retains both exact partition arrays even in shadow mode; control independently runs the full candidate set. A full early fallback can leave the complement empty. Empty phases emit a `skipped: true` receipt with group/stage and create no adapter process or synthetic attempt; retain the plan as membership evidence.

Relevant-now preserves portfolio order. Later and full controls use deterministic topological dependency order; the reference adapter executes requested tests serially. Cross-group dependencies are rejected until a dispatcher can enforce interleaved group work. CI must serialize whole group jobs sharing reviewed resource capacity; the command cannot enforce capacity across separate workers. Keep reviewed setup, credentials, immutable checkout and environment provisioning outside Argus. Adapter commands are never taken from artifacts. V2 adapters must preserve requested serial order; older strict adapters reject v2 requests and must be upgraded before opt-in.

## Evidence, failures and retries

Adapter request v2 carries stage plus schedule/episode/group/environment/mode provenance. Result format stays `argus.dev/functional-api-adapter-result/v1`: its test outcomes, failure details, artifacts and exact-result correlation are unchanged. Missing, duplicate, substituted or extra results, invalid JSON, timeout, cancellation and nonzero process exit cannot become successful normalized evidence. Failed, skipped and error test results produce failed/incomplete/error attempts; the runner writes valid evidence before returning a nonzero test-status error. Save stdout even on test failure.

Attempts use `argus.dev/execution-attempt/v2` and retain immutable provenance. Existing ingestion accepts both explicit versions:

~~~sh
go run ./cmd/execution-evidence ingest -file attempt-v2.json
~~~

Exact evidence retries are idempotent, including provenance and normalized results. Reusing an attempt ID for different content returns a conflict; a genuine rerun needs a new ID. Stored results and provenance are read from one repeatable-read snapshot. Ingestion validates evidence shape; trusted CI remains responsible for supplying genuine runner evidence. V1 shadow-report consumers deliberately reject v2; use the [staged safety reader](predictive-safety.md) for aggregate observations. Do not interpret an individual passed stage as complete coverage.

## Deployment and verification

Apply `20261021211703_add_predictive_api_stages.sql` with the existing migration command before enabling v2. It adds nullable provenance columns and one version-specific envelope constraint; old rows keep NULL metadata and their original retry fingerprints. No data backfill, row rewrite, new index or evidence mutation is required. Constraint validation scans the attempt header table and ALTER TABLE holds locks; schedule migration during a suitable window for the deployed table size. Old writers remain compatible, but old readers cannot interpret v2 rows. Deploy updated readers before opt-in and route legacy binaries only to legacy evidence.

Behavior rollback disables `-schedule`, restores the selected/full-suite workflow and retains the forward schema and immutable v2 evidence. Do not edit applied migrations or rewrite attempts. A failed transactional migration rolls back and can be retried normally. The schema adds no credentials or new retention boundary; existing evidence access/retention policy applies.

Run `make generate-contracts`, focused execution/architecture tests, `make api-validate`, `make db-validate` and `make validate`. Tests cover staged/shadow membership, mandatory controls, source mismatch, empty stages, incomplete results, strict v1 compatibility, provenance readback, concurrent retries, conflicts and database constraints. See [ADR-0048](../decisions/0048-opt-in-functional-api-staged-execution.md) and the [shared invariants](staged-execution.md).
