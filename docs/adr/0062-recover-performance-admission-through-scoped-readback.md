# ADR-0062: Recover performance admission through scoped readback

- Status: Proposed
- Date: 2026-10-26
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Recover using the original frozen request and independently configured provider.
- Return scoped retained evidence before lookup; settle only through existing fences.
- Leave unresolved work intact, without submission, retries or worker activation.

## Decision and consequences

PR8c1 supplies a lookup-only application operation with consumer-owned provider and
storage ports in requestintent. The Perfeng HTTP adapter checks scope and retained
child metadata before network access, then validates complete response correlation.
Existing owned outcome values and fenced PostgreSQL methods need no schema changes.
Network I/O never runs inside a storage transaction. Request expiry permits lookup;
only the database determines live settlement ownership.

An existing scoped outcome resolves admission without another provider exchange.
After a settlement error, one scoped read can recover committed evidence, including
another owner's disposition. Failed readback preserves the settlement error.
Cancellation returns no success evidence even if a write committed. Neither errors
nor lookup absence prove non-admission or authorize resubmission. No lease is created,
renewed or released here. Unsubmitted work absent at the provider remains unresolved.

PR8c2 retains initial-send/crash policy and opt-in worker composition; PR9 retains
ordered lifecycle. The recovery API alone activates no runtime and grants no new
privileges. Rollback removes callers, retaining immutable work and outcomes. Verify
frozen correlation, scoped replay, lost acknowledgements, stale ownership, absence
and cancellation through focused tests and `make validate`.
