# ADR-0066: Project performance lifecycle by provider revision

- Status: Proposed
- Date: 2026-10-27
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Correlate pinned provider snapshots to the exact frozen request before projection.
- Order by provider revision, rejecting conflicting histories and terminal mutation.
- Keep execution lifecycle separate from admission, analysis verdicts and authority.

## Decision and consequences

PR9a adapts the lifecycle rules from the already pinned public protocol. The
Perfeng adapter creates owned inward observations only after complete request
context, expected provider and run-issuer checks. Callers independently authenticate
provider/principal provenance. The exact frozen request digest binds each history.

`ProjectLifecycle` validates 1..1000 observations and sorts a copy by revision,
then event ID for deterministic equal-revision selection. Equal state/revision
snapshots may have new envelopes; reusing an event identity requires identical
canonical content and inward metadata. Equal revisions cannot change state.
Increasing revisions permit repeated nonterminal states or forward progression,
including skipped states. Terminal snapshots cannot change, even to a higher
revision with the same state. Conflicting old evidence rejects the whole projection.

Provider occurrence time is separate from delivery order. Late evidence remains
valid; a deadline or cancellation intent does not prove termination. COMPLETED
never establishes quality, SLO or regression PASS. This adds no persistence, I/O,
dependency, wire or worker change; rollback is a code revert. Durable consumption
and decision evidence use this foundation in the [PR9 plan](../development/plans/m09-pr9-performance-lifecycle.md).
