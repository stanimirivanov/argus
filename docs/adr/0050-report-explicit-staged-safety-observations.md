# ADR-0050: Report explicit staged safety observations

- Status: Proposed
- Date: 2026-10-22
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## Context

API and browser runners now retain immutable v2 stages. Existing v1 shadow
consumers intentionally reject those protocols. Monitoring must distinguish
complete comparison from missing/late/unknown results and avoid turning
observational measurements into promotion authority. Existing evidence lacks
criticality, billing, arrival telemetry and per-test completion timestamps.

## Decision

Add an explicit file-based safety archive index and observational report v1.
Reuse runner source validators to check the original portfolio, independent
manifest/discovery, reviewed bindings and complete plan before correlating v2
attempts. Preserve v1 shadow interfaces. Keep domain measurement logic independent
of archive paths, schemas and runner application packages; adapters project
validated evidence into the predictive domain.

Shadow compares early work with a complete independent control. Staged compares
early with its disjoint completed complement. Missing or late phases remain
pending; skipped/error labels remain incomplete. Undefined recall, calibration
or timing stays null with reasons. Reject wrong scope, substituted discovery or
inventory, duplicate attempts and invalid ordering rather than returning partial
apparently complete evidence.

Bind an explicit time window/as-of cutoff and retain input, schedule, plan,
manifest, policy and optional calibration/model provenance. Observed failure time
uses stage completion conservatively. Calibration drift compares complete live
labels with matching frozen probabilities and a prior repository/family baseline;
never reuse historical prediction labels as new outcomes. Resource costs remain
estimates with setup charged per nonempty group phase. Record unsupported
measurements explicitly; do not invent criticality or monetary rates.

## Consequences

The command is additive and needs no migration or alerting service. CI owns
input authentication, genuine runner origin and archive retention. Reporting
cannot replace obligatory controls or authorize release/promotion. Owner policy
remains M08-PR13; production alerting remains M10. Detailed denominators, window
recovery and estimate limitations are in the [guide](../integrations/predictive-safety.md).
Rollback stops new reporting without mutating retained evidence.

The original plan's focused predictive target was absent. Add its discoverable
alias now; existing verify already tests the same packages, so do not add a
second identical execution to CI. This slice remains one observational capability
rather than widening legacy shadow semantics or adding speculative persistence.

## Verification

Run `make predictive-validate`, real API/browser campaigns and `make validate`.
Regressions cover mixed families, empty/missing/late phases, unknown controls,
zero failures, exact misses, drift, setup/resource accounting, stage-level timing,
archive digests/paths, inventory substitution and strict contract decoding.
