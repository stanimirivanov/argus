# ADR-0052: Pin Perfeng catalog validation

- Status: Proposed
- Date: 2026-10-23
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## Context

Performance catalog synchronization must reject partial, stale, wrongly scoped or
inconsistent views before publishing evidence. Schema validation alone cannot prove
pagination completeness, resource closure or lifecycle consistency. Perfeng owns
the shared contract; importing its TypeScript implementation would couple product
dependency graphs and blur ownership.

## Decision

Pin the published performance-catalog/v1 schema and conformance fixtures at an
immutable upstream commit, with byte hashes checked by tests and Apache-2.0 license
retained. Validate locally with the existing JSON Schema dependency in an explicit
performance contract adapter. Implement complete-snapshot semantics against caller-
supplied trusted scope and clock, rejecting ambiguous JSON and invalid Unicode.
No provider implementation, execution authority or new dependency enters Argus.

## Consequences and compatibility

This additive foundation leaves existing test-catalog v1 and all persistence and
execution behavior unchanged. It is consumed by the upcoming HTTP synchronization
and PostgreSQL publication boundaries in M09-PR4b and M09-PR4c. The split keeps network and
transaction semantics out of this contract review. Scope/digest validity does not
authenticate payloads or attest the referenced producer artifacts. The caller must
own input bytes and recheck freshness before later publication or eligibility.

## Verification and rollback

Run focused contract and architecture tests plus `make validate`. Pin conformance
fixtures, test the canonical resource digest independently, and exercise incomplete
pagination, mismatched scope, expiry, reference substitution and retirement. There
is no runtime integration to disable or database migration to reverse in this slice.
See the [integration guide](../integrations/perfeng-catalog.md) for limits and follow-up.
