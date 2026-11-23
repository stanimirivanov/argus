# ADR-0075: Poll cancellation with durable baseline cooldown

- Status: Proposed
- Date: 2026-10-28
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- One callable step selects at most one admitted cancellation under trusted scope/issuer.
- Commit baseline cooldown before provider I/O; longer valid retry hints extend it.
- Preserve cursor progress and uncertain errors; runtime activation remains PR10d2c.

## Decision and consequences

PR10d2b adds `perfeng.CancellationPoller.One` over the existing due-selection,
monotonic deferral and authenticated cancellation consumer. A positive whole-second
interval is independently configured. Failed/unknown baseline deferral permits
no provider effects. Successful baseline retention survives a crash or generic
provider error. Longer whole-second HTTP 429/503 RetryAfter hints extend the
deadline; extension failure preserves both errors for recovery.

Each step selects once, then performs at most one GET and one cancellation POST.
No sleeps, internal retries, original submits or generated identities. Return an
exclusive cursor even on candidate errors, including usable partial reads; only
end-of-traversal resets it, without an immediate second scan. Nonadvancing or
mismatched work fails closed. A missing position on read failure cannot advance.

This is eligibility control, not ownership: already selected/in-flight calls
cannot be revoked. Concurrent pollers can duplicate cancellation; serialize calls
in the embedding root. No schema/config/wire/dependency changes or worker activation.
Rollback reverts the callable API while retaining ADR-0074 cooldown state. PR10d2c
owns opt-in composition, scheduling and admission recovery coordination.
