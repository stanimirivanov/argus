# ADR-0083: Pin request-linked analysis publications

- Status: Proposed
- Date: 2026-10-30
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Pin the published Apache-2.0 performance-analysis/v1 envelope separately from lifecycle.
- Preserve original context, explicit native bindings and immutable artifact claims.
- Codec validity grants no admission, artifact-fetch or decision authority.

## Decision and consequences

PR9d2a copies the schema/example byte-for-byte from perfeng-contracts commit
cd2442f433ce93d9a886c14ad6ff8aa8488a623b, bundle 0.25.0, under the existing
snapshot license. Existing native analysis/catalog/integration pins remain intact.
Reuse bounded unique-key Unicode JSON, numeric resource controls and offline schema
validation without a new dependency or external resource loader.

The separate owned envelope preserves public scoped Run and native Run/test/workload/
policy identity, explicit nullable baseline identity/reference and exact numeric size
syntax. Enforce original context/time, source/scoped Run issuer, report/native Run
and nullable-baseline relationships. Credential-bearing or mutable artifact URLs
are rejected. Late publication after deadline is valid and grants no verdict.
Do not widen the existing integration union or infer native identities from labels.

This shared codec foundation supports later authenticated reference retrieval and
reviewed correlation. Those steps must authenticate full scope, compare admitted
context/run and frozen native resource/policy/baseline/rule associations, and verify
the exact analysis bytes by checksum/size. PR9d3 owns durable decision evidence and
opt-in runtime consumption. No fetch, raw result reads, persistence, migration,
blocking, external statuses or overall PASS are introduced. Revert the unused codec
before consumers without changing existing runtime behavior.
