# Perfeng catalog validation

## TL;DR

- M09-PR4a validates the complete public `performance-catalog/v1` projection offline.
- Supply trusted provider, principal, producer version and observation time separately.
- Validation establishes consistency, never authentication or launch permission.
- HTTP synchronization and atomic PostgreSQL publication remain M09-PR4b/PR4c work.

## Pinned boundary

The consumer-owned adapter is
[`internal/performance/adapters/perfeng`](../../internal/performance/adapters/perfeng).
It embeds the published JSON schema from
[`perfeng-contracts` commit 041273947703542f7fe22eab838c176df5517974](https://github.com/stanimirivanov/perfeng-contracts/tree/041273947703542f7fe22eab838c176df5517974),
bundle 0.24.0. The provider reference implementation is
[`perfeng-control-plane` commit c1893b30d6a03eac346463d667b020f4430c74b1](https://github.com/stanimirivanov/perfeng-control-plane/tree/c1893b30d6a03eac346463d667b020f4430c74b1).
No provider implementation or TypeScript sources are imported. Existing
`jsonschema/v6` validates the schema locally; no dependency is added.

Copied schema and fixture bytes live in the adapter's `snapshot/` directory.
`TestPublishedArtifactsRemainPinned` checks upstream SHA-256 values; updates require
reviewing the upstream contract and updating provenance and conformance tests together.
The upstream Apache-2.0 license accompanies those artifacts. This is an external
contract pin, not an Argus Effect Schema source or generated test-catalog contract.

## Validation and caller responsibilities

`Validate` accepts an ordered set of raw JSON pages, independent `Expectations`
and a nonzero observation instant. It checks schema and unknown fields, unambiguous
JSON and Unicode, matching snapshot metadata, cursor continuity, page/item counts,
canonical ordered-resource SHA-256, issuer/principal/producer scope, freshness,
unique current identities, exact reference closure, reciprocal workload/profile
membership, active dependencies and approved baseline context. Resource identity
includes kind and issuer; names never authorize a mapping.

The provider contract permits at most 1,000 pages and 100 resources/page. Argus
additionally limits input to 2 MiB/page, 64 MiB/snapshot and 32 JSON nesting levels.
Complete empty snapshots and explicit retirement history are valid. A null baseline
is explicit absence, not implicit approval. Errors omit provider-controlled values.

The caller owns immutable page bytes during validation. Successful validation
publishes no state and grants no execution eligibility. Authentication must establish
the independently configured principal scope; the digest proves consistency rather
than external artifact provenance. Recheck freshness at eventual publication/use.
Expiry is exclusive, generation inclusive. Fixtures use the frozen upstream instant
2026-10-22T12:00:00Z; production callers must supply their actual trusted clock.

## Remaining synchronization slice

M09-PR4b consumes this validator in bounded authenticated HTTP pagination and
cancellation with an operator inspection command. M09-PR4c consumes it again at
the durable import boundary, adds transactional publication through a performance-only
store and additive migration, and connects synchronization to publication. It must
retain the prior view/history on refresh failure, reject identity conflicts and
older concurrent publication, and handle idempotent replay. These are the remaining
slices of M09-PR4, not new milestone features.
Missing resources in a complete refresh cease eligibility; failed or stale
refreshes must never erase prior evidence or silently grant fresh eligibility.
No workload selection or execution is introduced here.

## Verification

Run `go test -vet=off ./internal/performance/adapters/perfeng ./internal/architecture`
and `make validate`. Database verification belongs to M09-PR4c; this foundation
does not touch persistence. See [ADR-0052](../decisions/0052-pin-perfeng-catalog-validation.md).
