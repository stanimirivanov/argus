# Staged execution safety and cost

## TL;DR

- `predictive-safety` compares explicitly archived v2 API/browser attempts with
  independently verified plans, portfolios, manifests and reviewed bindings.
- Shadow requires early plus an independent full control; staged requires early
  plus the exact remaining complement. Empty memberships need no attempt.
- Missing, late, skipped or errored evidence cannot produce complete recall.
  Zero failing controls give null recall, not a fabricated perfect score.
- Metrics are observations, never promotion approval. M08-PR13 owns owner policy;
  M10 owns production alerting. Existing v1 shadow consumers remain strict.

## Archive and trust

Create an `argus.dev/predictive-safety-index/v1` file. Every file reference has a
relative `path` and SHA-256 of its complete raw bytes. Paths must stay beneath
its directory after symlink resolution; absolute paths and backslashes fail.
The index and each document are bounded to 16 MiB, total reads to 256 MiB,
100 runs and 100,000 candidate exposures. Retain the index and all referenced
files with the report: the output pins the index digest, original plan digest,
schedule, manifest, selection policy version and explicit attempt identities.

~~~json
{
  "apiVersion": "argus.dev/predictive-safety-index/v1",
  "windowStart": "2026-10-22T09:00:00Z",
  "windowEnd": "2026-10-22T11:00:00Z",
  "asOf": "2026-10-22T10:30:00Z",
  "runs": [{
    "id": "orders-run", "episodeId": "episode", "mode": "shadow",
    "schedule": {"path": "portfolio.json", "sha256": "<full-file-sha256>"},
    "manifest": {"path": "manifest.json", "sha256": "<full-file-sha256>"},
    "bindings": {"path": "bindings.json", "sha256": "<full-file-sha256>"},
    "plan": {"path": "plan-v2.json", "sha256": "<full-file-sha256>"},
    "attempts": [
      {"path": "early-v2.json", "sha256": "<full-file-sha256>"},
      {"path": "control-v2.json", "sha256": "<full-file-sha256>"}
    ]
  }]
}
~~~

Replace digest placeholders with actual lowercase SHA-256 values. Browser runs
also require `discovery`, referencing the complete browser-discovery/v1 file.
An optional `calibration` reference supplies predictive-calibration/v1 evidence.
API manifests retain their runner's canonical digest inside plans/attempts;
archive references separately pin raw bytes, so harmless JSON whitespace does
not change the established API manifest identity. Browser manifest/discovery
hashes remain raw-byte identities.

~~~sh
go run ./cmd/predictive-safety -index archive/index.json > safety-report.json
~~~

Original inputs are checked with the same source/plan validators as the
[API](functional-api-staged.md) and [browser](browser-staged.md) runners. Every
reviewed group remains visible even when no attempt exists. Attempts are supplied
explicitly, never discovered by recency; duplicate IDs, ambiguous duplicate
schedule/episode runs, extra phases, wrong revisions/scopes, substituted browser
inventories, discovery mismatches and overlapping/incompatible phases fail the
whole command without output. Inputs authenticate no caller: genuine runner
origin, artifact retention and reviewed configuration remain trusted CI duties.
Artifact reference shape is validated, but reports do not reread expired trace
files or rerun tests. The runners remain responsible for artifact-byte checks.

## Window, recovery and completeness

Attempts must start within `[windowStart, windowEnd)` and after their decision
feature cutoff. Results completed after `asOf` or `windowEnd` are late: their
labels and measured duration are excluded. As-of time is explicit; no local clock
or implicit latest lookup changes a report. Artifact arrival time is not recorded,
so completion time is the observation proxy, not an ingestion-latency measure.

| State | Meaning |
|:--|:--|
| complete | Every required nonempty phase is present, in order, with passed/failed labels. |
| pending | A required phase is missing or late. |
| incomplete | Phases arrived, but skipped/error outcomes prevent complete comparison. |
| empty phase | The independent plan assigns zero tests; no attempt is permitted. |

Recover by adding the exact missing immutable attempt or extending the explicit
window/as-of snapshot, then rerun. Do not relabel skipped/error tests as passes.
A genuine rerun has a new attempt ID; choose one explicit execution per episode
rather than merging retries into extra independent samples. A report contains no
safe/approved flag and cannot bypass mandatory later/control execution.

## Measurement semantics

- A miss is a failing full-control/remaining test not failed early. Shadow misses
  distinguish not-selected from selected-but-not-reproduced. Staged labels are
  the disjoint union of early and remaining results; shadow labels come from
  the independent full control. Non-reproduced failures are observations, not
  proof of a selector defect or a diagnosis of flakiness.
- Confirmed misses remain visible in partial reports; missing/unknown early
  outcomes cannot prove a selected test was missed. Late labels are excluded.
- Recall and failure counts require complete evidence. Zero failures leave
  recall undefined. Repository/family cohorts aggregate candidate exposures and
  early selection, not globally unique tests; any incomplete group withholds
  aggregate failure recall. Criticality is unavailable because source contracts
  do not retain that classification.
- Brier score and decile expected calibration error require complete labels and
  exact per-test probabilities matched to the frozen calibration and portfolio.
  Training/calibration split IDs and calibration source must match the portfolio.
  Portfolio v1 does not retain its original model digest: authenticated CI must
  supply the original matching calibration artifact, not substitute an arbitrary
  model with coincidentally equal probabilities. This limitation remains visible.
  Model parameters are hashed separately; the complete calibration artifact is
  also pinned. Its holdout must end before the monitoring window. Historical
  prediction labels are ignored. Drift is signed live ECE minus matching
  repository/family baseline ECE; absent comparable evidence gives null drift.
- Each phase reports summed test duration and reported attempt elapsed time
  separately. Estimated duration adds reviewed setup once per nonempty group
  invocation. Estimated resource-unit milliseconds add test estimates times
  reviewed units plus setup times reviewed environment capacity. This is neither
  billed cost nor measured resource consumption. Sharing setup across CI workers
  is outside this estimate; do not sum it as an independently measured cost.
- First observed failure uses the earliest completed failing attempt relative
  to the first nonempty phase's start. It is a conservative stage-level point;
  per-test completion cannot be reconstructed from result order or durations.
  An observed early failure is available while later work is pending. Missing
  early origin or no observed failure leaves the metric null.
- Missing calibration, billing, measured resources, arrival telemetry and
  analysis/review overhead are reported unavailable rather than invented.

## Verification and rollout

Run `make generate-contracts`, `make predictive-validate`, `make api-validate`,
`make ui-validate` and `make validate`. The focused predictive target exposes the
existing predictive package tests; `make verify` already runs them through its
full Go test gate, so CI does not execute that alias as a duplicate suite.

This is an additive file-based command and versioned report; it changes no
runner, database schema or promotion policy. Deploy the reader after the v2
runner protocols. Rollback stops producing the new report and retains archived
inputs/evidence. See [ADR-0050](../decisions/0050-report-explicit-staged-safety-observations.md).
