# ADR-0069: Consume authenticated lifecycle snapshots

- Status: Proposed
- Date: 2026-10-28
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Require explicit lifecycle opt-in under the existing authenticated worker scope.
- Traverse one settled candidate per tick; validate history before provider GET.
- Resolve uncertain persistence only through exact scoped event readback.

## Decision and consequences

PR9c adds `IntegrationHTTP.ConsumeLifecycle` and issuer-filtered `NextLifecycle`.
The existing serial worker may consume one lifecycle candidate after admission
processing on each tick. `ARGUS_PERFORMANCE_DISPATCH_LIFECYCLE_ENABLED=true`
opts in; dispatch itself must already be enabled. Endpoint, credentials, provider,
principal, request issuer and polling interval retain their existing independent
operator authority. Retained messages cannot reroute I/O or grant authorization.

Traversal considers only accepted/lifecycle settlements and rotates an in-memory
position cursor, resetting at the end. A failed candidate does not prevent later
positions from being visited. Restart safely revisits evidence; multiple workers
may perform duplicate GETs, while atomic storage guards serialize appends. Each
call bounds HTTP and storage independently, propagates cancellation, and performs
no network access in a transaction. The existing worker joins before pool closure.

Canonical admission, frozen context and stored history are revalidated. Terminal
history skips GET; terminal recovery evidence seeds its own durable anchor without
a new lookup. Expiry permits late readback, never inferred termination. Provider
failure, absence, changed run and contradictory snapshots remain unresolved. Lost
append acknowledgement permits one history read; success requires complete exact
event equality, not a newer revision. No POST, immediate retry or new identity is
introduced. Logs disclose only retained/unresolved classifications.

No schema, wire, dependency or default activation changes. Disable lifecycle
polling or revert application code to roll back while retaining history. Traversal
still visits terminal positions to validate their evidence; a durable fair scheduler,
fleet throttling, retention and richer operator recovery belong to M10. PR9d owns
analysis verdicts; lifecycle completion never implies PASS.
