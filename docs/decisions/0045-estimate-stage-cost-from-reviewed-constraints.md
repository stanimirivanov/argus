# ADR-0045: Estimate stage cost from reviewed constraints

- Status: Proposed
- Date: 2026-10-21
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Estimate duration from the nearest-rank p90 of completed training controls, with an explicit reviewed cold-start value.
- Require complete, timely reviewed resource, environment, and dependency declarations bound to the decision manifest and cutoff.
- Report serial estimates and dependency/resource lower bounds over all candidates; later scheduling owns portfolio selection.

## Context

Predictive observations retain control durations and environment identity, but do not measure resource use or declare test dependencies. Reusing held-out durations would leak future information into a scheduling estimate. Guessing missing resource and dependency metadata would conceal assumptions that affect feasibility.

## Decision

`predictive-cost` accepts the existing chronological archive and an additive `argus.dev/predictive-stage-constraints/v1` review document. Every held-out episode needs exactly one review with review ID, reviewer, review time, immutable manifest digest, feature cutoff, complete candidate membership, environment capacities/setup times, per-test resource units, dependencies, and a cold-start duration. Review time must be strictly before the feature cutoff; the manifest and cutoff must match the archived decision. A non-null observed environment must match the reviewed environment. When the observation has no environment, the review supplies its explicit scheduling label.

Only eligible training episodes whose controls completed before holdout start supply duration samples. For each stable test ID, sort positive millisecond samples and take nearest-rank p90 (`ceil(0.9*n)`, one-based). Clamp measured zero durations to one millisecond. Without samples, use the positive reviewed cold-start value. Report sample count and source; estimates carry no statistical guarantee. Held-out outcomes, durations, and other held-out episodes never update the estimator.

Resource units are abstract parallel slots within each environment, not inferred CPU or memory measurements. Each environment has 1–10,000 slots and one reviewed setup duration. Reject unknown/duplicate candidates, unknown environments, demand above capacity, and unknown, repeated, self, or cyclic dependency edges. Dependencies mean prerequisites must finish before the dependent test starts. Environment setup can begin at stage start independently of other environments.

The additive `argus.dev/predictive-stage-cost/v1` report binds the exact review file bytes by SHA-256 and includes all reviewed constraints with the duration estimates. Serial cost is the sum of estimated durations plus setup once for each used environment. Per-environment resource work is `sum(durationMs * resourceUnits)`; its lower bound is setup plus ceiling work/capacity. The dependency lower bound is the longest prerequisite path, accounting for environment setup readiness. The stage lower bound is the maximum of these bounds. These bounds apply to estimated costs and do not assert a feasible schedule or actual runtime.

## Alternatives considered

A fixed duration default would hide repository-specific cold-start assumptions. Latest-only duration estimates are more sensitive to single fast runs; p90 is a deterministic conservative baseline. Inferring resource or dependency declarations from names would create unreviewed policy. A full scheduler belongs to the next portfolio slice.

## Consequences and compatibility

Review identities and timestamps are declared provenance, not signatures or proof of human approval. Source CI must authenticate the retained review and archive. A changed review file changes its digest even when only formatting changes. Existing observation, replay, ranking, calibration, and abstention contracts remain unchanged. Changing quantile, setup, resource, or dependency semantics requires a new contract version.

## Rollout and validation

The CLI is offline and dispatches nothing. Input and output are capped at 16 MiB, with at most 1,000 episodes, 10,000 tests per episode, 100 environments per review, and 100 dependencies per test. Domain and CLI tests cover quantiles, cold start, chronology, holdout-label independence, input ordering, review binding, graph/resource validation, and no partial output. Revert the new command and contracts together if these semantics change.
