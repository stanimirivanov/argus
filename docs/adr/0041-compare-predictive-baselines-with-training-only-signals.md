# ADR-0041: Compare predictive baselines with training-only signals

- Status: Proposed
- Date: 2026-10-20
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Compare five deterministic policies on the same held-out candidates.
- Use only replay-declared training history for prior failures and duration estimates.
- Apply one estimated-duration budget to four policies; run-all is the unbudgeted reference.
- Read held-out outcomes and durations only after selection to measure misses and feedback cost.

## Context

ADR-0040 produces reproducible holdout episodes and eligible histories, but it does not define a comparison policy. Using actual held-out durations to fit a budget or failure labels to choose a test would leak the answer into a baseline. Comparing policies at different budgets would also obscure their trade-offs.

## Decision

`predictive-baselines` reads the same explicit dataset index as replay and emits `argus.dev/predictive-baselines/v1`. Stable test identity is a SHA-256 of repository provider/host/ID, family, adapter, project, environment, suite key, and test key. Revision is deliberately excluded so earlier controls can inform a later test revision. The archive retains human-readable coordinates.

Run-all selects every test in stable identity order and ignores the budget. Explicit-impact considers only `RUN_REQUIRED` candidates. Recent-failure ranks candidates by the most recent eligible training episode in which they failed. Duration ranks by the latest eligible training duration, shortest first. Seeded-random ranks by SHA-256 of the explicit decimal seed and stable test identity. Ties use identity order. The four constrained policies greedily take each candidate that fits the remaining estimated budget and continue past candidates that do not fit. A test without training history receives a 1,000 ms estimate.

The evaluator fixes all selected identities and order before reading held-out outcomes or durations for metrics. It reports estimated and actual selected time, full-suite time, selected and missed failure counts, and sequential time to first selected failure. A missing failure yields null time-to-first-failure. Actual time may exceed the estimated budget; that difference is evidence for later estimation work.

## Consequences

The report is an offline baseline, not a production scheduler or promotion threshold. A latest-duration estimate is deliberately simple; dependency, resource, environment, calibration, uncertainty, and learned ranking belong to later M08 slices. Source CI remains responsible for completeness and authenticity of the indexed archive. Baseline inputs are bounded to 100,000 tests across the index and 10,000 per episode.

## Compatibility and migration

The contract and command are additive. Existing observation and replay envelopes remain unchanged. A change to ordering, fallback cost, or metric meaning requires a new baseline contract version.

## Validation

Focused tests prove deterministic ordering, equal budget semantics, missed-failure accounting, held-out-label independence, duplicate-test rejection, archive-order independence, and portable schema validation. Repository verification and race tests cover integration.
