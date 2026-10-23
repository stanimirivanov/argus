# Perfeng catalog fetching and inspection

## TL;DR

- M09-PR4a validates the complete public `performance-catalog/v1` projection offline.
- Supply trusted provider, principal, producer version and observation time separately.
- Validation establishes consistency, never authentication or launch permission.
- M09-PR4b adds authenticated fetching and read-only operator inspection.
- Atomic PostgreSQL publication remains M09-PR4c work.

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

## Authenticated operator inspection

Inject `ARGUS_PERFENG_TOKEN` through the operator's secret environment. The provider
must explicitly grant that credential `catalog:read`. Credentials are never CLI
arguments. Independently review the endpoint, provider issuer, principal issuer/ID,
producer ID and accepted version; do not derive them from a fetched page.

```sh
go run ./cmd/performance-catalog -endpoint https://perfeng.example/v1/performance-catalog -issuer https://provider.example/identity -principal-issuer https://auth.example/tenant -principal-id argus-service -producer-version 1.0.0
```

`-producer-id` defaults to `perfeng-control-plane`; other scope/version options are
required. The endpoint is the full reviewed URL, including any deployment prefix.
It must use HTTPS; development HTTP accepts literal loopback IPs only, not DNS
names. Embedded credentials, queries and fragments are rejected. TLS uses the
normal trusted roots; the CLI provides no TLS verification bypass.

The command writes a JSON array of complete validated pages to stdout after success.
Help/version work without credentials. Exit codes are 0 for success, 1 for fetch,
validation or output failures, and 2 for invalid command syntax. Failed fetches
write no snapshot bytes. There is no database connection, background refresh,
eligibility publication or workload launch. Shell output redirection is not atomic
publication; do not use an existing evidence file as the direct redirection target.

All redirects are rejected, including same-origin redirects. Cookies are disabled;
every page uses the same bearer credential and configured endpoint, with the opaque
cursor safely encoded as its only query parameter. Enumeration is limited to 1,000
pages/attempt, 2 MiB/response body, 64 MiB across attempts, 10 seconds/request and 30
seconds overall. Cancellation covers the operation and response reads. Freshness
is checked with the actual clock after fetching the complete view and again after
validation, before returning pages.

One HTTP 410 restarts enumeration from the initial endpoint and discards all prior
pages; a second 410 fails. Provider caches are process-local, so deployments need
instance affinity during pagination that does not depend on cookies, such as a
reviewed endpoint routed to one instance. HTTP 401/403 requires credential/scope review;
404 requires endpoint/publication configuration review. HTTP 429/503 is reported
without automatic retry: retry later according to deployment policy. Timeouts,
malformed, mixed, partial or stale responses produce errors without exposing remote
bodies, credentials or cursor URLs. Failures never represent an empty catalog.

## Remaining publication slice

M09-PR4c consumes the same validator at
the durable import boundary, adds transactional publication through a performance-only
store and additive migration, and connects synchronization to publication. It must
retain the prior view/history on refresh failure, reject identity conflicts and
older concurrent publication, and handle idempotent replay. These are the remaining
slices of M09-PR4, not new milestone features.
Missing resources in a complete refresh cease eligibility; failed or stale
refreshes must never erase prior evidence or silently grant fresh eligibility.
No workload selection or execution is introduced here.

## Verification

Run `go test -vet=off ./internal/performance/adapters/... ./internal/architecture`
and `make validate`. Executable metadata/exit smoke tests cover the inspection
command. HTTP tests exercise loopback authentication, opaque cursors, bounded
restart, redirects, cancellation, stale/wrong scope and invalid/oversized responses.
Database verification belongs to M09-PR4c; this slice does not touch persistence.
See [ADR-0052](../decisions/0052-pin-perfeng-catalog-validation.md) and
[ADR-0053](../decisions/0053-fetch-authorized-catalogs-for-inspection.md).
