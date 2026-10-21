# ADR-0046: Schedule diverse budgeted portfolios

- Status: Proposed
- Date: 2026-10-21
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Compose reviewed cost estimates and the abstention gate into an offline serial schedule.
- Preserve mandatory prerequisite closure and full-control fallback even when budget is exceeded.
- Prefer uncovered capabilities, then fill by calibrated failure risk per duration.

## Context

Cost lower bounds do not demonstrate a feasible schedule. Predictive selection must preserve hard policy and uncertain decisions, admit dependencies and environment setup, and make every omission visible. Introducing a parallel optimizer here would enlarge the correctness and review surface before an interpretable baseline exists.

## Decision

Use the deterministic serial selection policy in the [portfolio guide](../integrations/predictive-portfolio.md). Required tests and their complete prerequisite closure precede electives. Diversity is a first pass over risk/duration-ranked candidates; ranked fill follows. Admit a closure only when its entire marginal serial cost fits. Charge setup once on first environment use. Serial execution satisfies individually validated environment capacities and dependency ordering. It sacrifices parallel opportunities but never infers feasibility from a lower bound.

The existing abstention gate overrides budget with full control. Bound optimization work to one million visits/inspections per episode; limit exhaustion discards partial work and also requires full control. Expose both modes and overruns explicitly. No held-out control label or duration enters selection or timing. Immutable decision-time capabilities are selection metadata, not identity.

## Alternatives considered

A parallel optimizer can improve utilization but requires additional admission, resource occupancy and correctness proofs. Risk-only selection misses capability diversity. Dropping required tests to fit budget violates the safety policy. Truncating a report would hide missing candidates; reject oversized output instead.

## Consequences and compatibility

The command and portable contract are additive; previous observation, review, ranking and abstention formats remain unchanged. Reports retain complete cost/review and abstention evidence, selected serial offsets/reasons, required closure and omitted candidates. This is an estimated feasible baseline, not a wall-time guarantee or global optimum. Full-control capability summaries are intentionally empty. Original hard decisions and capability authenticity still require the retained archive.

No production dispatch, promotion or remaining-later orchestration is authorized. Later execution work owns explicit handling of omitted IDs. Removal of this command/contract is sufficient rollback; no migration is needed.

## Verification

Domain tests cover budget boundaries, diversity, ranked fill, setup, dependency closure, deterministic ties, required overruns, held-out independence and bounded-work fallback. CLI tests verify portable evidence and no partial output; semantic contract tests reject tampered timing, partitions, dependencies, capacity and fallback. Run `make validate` before merge.
