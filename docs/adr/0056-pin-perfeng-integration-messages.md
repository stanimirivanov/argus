# ADR-0056: Pin Perfeng integration messages

- Status: Proposed
- Date: 2026-10-25
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Adopt the provider's immutable shared integration contract without its runtime.
- Validate bounded owned transport values offline; claims grant no authority.
- Keep request creation, persistence and dispatch in their owning later slices.

## Decision and alternatives

Reuse the performance contract adapter and existing JSON Schema dependency for
performance-integration/v1 from contracts commit
`041273947703542f7fe22eab838c176df5517974`, bundle 0.24.0. Pin schema, OpenAPI and
exchange fixtures by SHA-256 and retain the adjacent upstream Apache-2.0 license.
This is the same generation adopted by Perfeng PR6. Importing producer Go or
TypeScript packages would couple independent product dependency graphs; duplicating
the shared schema in Argus Effect sources would create competing authorities.

Decode closed, bounded Unicode JSON with duplicate-key rejection, offline reference
resolution and published identity/time relationships. Preserve nullable baseline,
evidence references, explicit source algorithm/digest and exact revision syntax.
Outbound validation rejects lossy normalization. Return safe classified errors and
no partial output. No wall clock, credentials, URLs or provider calls are consulted.

## Consequences and verification

This additive foundation changes no existing schema, dependency, SQL, command or
execution behavior. Request compilation and dispatch/lifecycle adapters consume
the same boundary. Authentication, owner approval, artifact association, freshness,
cross-message context equality, replay and terminal ordering remain runtime duties.
Late lifecycle/cancellation messages are valid; acceptance at deadline is invalid.

Run pinned fixture/negative/ownership/concurrency tests and `make validate`.
There is no runtime activation or data migration to reverse. Future contract
updates require joint compatibility review, new pins and conformance verification.
