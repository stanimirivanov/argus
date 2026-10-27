# ADR-0064: Submit performance work only from first acquisitions

- Status: Proposed
- Date: 2026-10-26
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Acquire fresh ownership for each invocation; only fence one may submit once.
- Bind configured provider/principal and request issuer independently of wire claims.
- Recover reacquired or expired work through lookup, never automatic POST replay.

## Decision and consequences

PR8c2a2 adds a callable one-shot dispatcher over the existing claim, provider and
settlement boundaries. Previously approved immutable work is the input. Validate
provider configuration offline before consuming first-send eligibility. Filter both
issuers using PR8c2a1 acquisition so other issuers retain fence one. Check child
integrity and trusted time after acquisition; bound
submission by remaining request/lease time. Context bounds cannot make network and
database writes atomic: provider admission deadlines and database settlement fences
remain authoritative. Shared settlement/readback semantics preserve uncertain writes.

First-acquisition-only sending implements the agreed no-implicit-retry constraint.
Lost acquisition acknowledgement or crash before POST can strand absent work; a
lookup 404 proves nothing. Never reset fences or invent identities to recover it.
The provider's idempotency support does not independently authorize automatic replay.
Failed operations retain leases until expiry; no scheduler or release is introduced.

PR8c2b owns opt-in runtime composition and operator configuration; PR9/PR10 retain
lifecycle/cancellation. No schema, wire or privilege change and no worker activation
occurs here. Rollback disables callers and preserves evidence/fences. Verify focused
application/HTTP tests, database-backed first/reacquired dispatch, `make validate`
and `make db-validate`.
