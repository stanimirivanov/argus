# ADR-0074: Retain monotonic cancellation cooldown

- Status: Proposed
- Date: 2026-10-28
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Select admitted cancellation work under independent scope and request/attempt issuer.
- Persist positive whole-second cooldown using database time and an atomic maximum.
- Both cancellation and lifecycle selectors respect deadlines; selection grants no ownership.

## Decision and consequences

PR10d2a adds a mutable operational sidecar without rewriting immutable intent,
admission or history. `NextCancellation` uses an exclusive position cursor and
selects accepted/lifecycle outcomes with retained cancellation. `DeferCancellation`
requires the same scope, both issuers and admission. Concurrent or shorter updates
cannot shorten the deadline. PostgreSQL guards scope, binding, finite/bounded time
and monotonic updates; deletion and truncation are prohibited.

Database time avoids client-clock comparisons. An expired deadline remains stored
but eligible. Unknown commit results require conservative repetition/readback,
never assumed scheduling success. Cooldown is neither a lease nor provider receipt;
already selected/in-flight calls are not revoked. Direct callers own scheduling.

This is behavior-adjacent for `NextLifecycle`: a future cancellation deadline now
suppresses its GET too. Empty sidecars preserve selection behavior. Apply migration
21 before new selectors; no backfill. Retain state on rollback and stop older
pollers, whose selectors ignore cooldown. No wire/config/dependency or activation
change. PR10d2b owns bounded polling; PR10d2c owns runtime/recovery coordination.
