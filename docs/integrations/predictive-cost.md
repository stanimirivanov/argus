# Estimate predictive stage cost

## TL;DR

- Retain the complete [chronological archive](predictive-replay.md) and a reviewed constraints document for every holdout episode.
- Run `predictive-cost -index index.json -constraints constraints.json`.
- Inspect duration provenance, resource work, and time bounds before using the report for later scheduling.

The constraints document uses `argus.dev/predictive-stage-constraints/v1`. Each `episodes` entry has `id`, `reviewId`, `reviewedBy`, `reviewedAt`, `featureCutoffAt`, `manifestSha256`, `environments`, and `tests`. The review must precede and match the archived decision cutoff and manifest. Use the stable test IDs emitted by the existing ranking or abstention report. Include each held-out candidate exactly once.

Each environment declares `key`, `capacityUnits` (1–10,000 abstract parallel slots), and `setupDurationMs` (0–86,400,000). Each test declares `id`, `environment`, positive `resourceUnits` no larger than capacity, positive `coldStartDurationMs` up to 86,400,000, and `dependencies` containing prerequisite test IDs from the same episode. Use an empty array for no prerequisites. Unknown, duplicate, self, or cyclic edges are rejected. An observed environment must match the review; for an observation with a null environment, the review supplies its explicit scheduling label.

The `argus.dev/predictive-stage-cost/v1` output retains the reviewed constraints and records `constraintsSha256` over the exact input bytes. A test with eligible completed training controls uses nearest-rank p90 duration (`ceil(0.9*n)`, one-based), with zero measurements clamped to one millisecond. It reports `samples` and `source: "training-p90"`. An unseen test uses the reviewed duration and `source: "reviewed-cold-start"`. No held-out outcome or duration changes the estimate.

`serialDurationMs` sums estimated durations and setup once per used environment. `workUnitMs` is duration times resource demand, summed per environment. `resourceLowerBoundMs` is setup plus ceiling work/capacity. `dependencyLowerBoundMs` follows the longest prerequisite path with environment setup readiness. `lowerBoundMs` is the maximum of the dependency and resource bounds. Setup is assumed independently available from stage start; unused environments contribute no setup cost. These are estimates and lower bounds, not an executable schedule or a promise of wall time.

Malformed, incomplete, late, mismatched, or infeasible reviews produce no output. Both input and output are bounded to 16 MiB. Review provenance is a declaration; source CI must establish authenticity and retain its review record. This command does not select a portfolio or dispatch tests. See [ADR-0045](../decisions/0045-estimate-stage-cost-from-reviewed-constraints.md) for the policy and compatibility boundary.
