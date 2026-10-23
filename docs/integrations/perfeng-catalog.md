# Perfeng catalog synchronization and publication

## TL;DR

- M09-PR4a validates the complete public `performance-catalog/v1` projection offline.
- Supply trusted provider, principal, producer version and observation time separately.
- Validation establishes consistency, never authentication or launch permission.
- M09-PR4b adds authenticated fetching and read-only operator inspection.
- M09-PR4c adds opt-in atomic PostgreSQL publication and stored-current inspection.
- Apply migrations before publication; stored evidence grants no execution authority.

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
write no snapshot bytes. Default inspection opens no database connection. There is
no background refresh or workload launch. Shell output redirection is not atomic
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

## Atomic publication and stored inspection

Set `ARGUS_DATABASE_URL` through the deployment secret environment and apply
`go run ./cmd/migrate` with migration credentials before using database modes.
Then use the ordinary restricted runtime role. Roles require schema USAGE,
SELECT/INSERT on performance snapshots/resources, SELECT/INSERT/UPDATE on current,
and function execution. Do not grant runtime schema ownership, DDL or TRUNCATE.

```sh
go run ./cmd/performance-catalog -publish -endpoint https://perfeng.example/v1/performance-catalog -issuer https://provider.example/identity -principal-issuer https://auth.example/tenant -principal-id argus-service -producer-version 1.0.0
go run ./cmd/performance-catalog -stored -issuer https://provider.example/identity -principal-issuer https://auth.example/tenant -principal-id argus-service
```

`-publish` fetches all pages, revalidates them, then atomically stores immutable
evidence, relational resource identities/lifecycle and the scoped current pointer.
Output is `{"created":true,"pages":[...]}` from durable readback. Exact replay
returns `created:false` and never repoints current. The store also accepts exact
historical/expired replays; live fetching/import still requires fresh source pages.
A snapshot ID is immutable within provider/principal scope: changes to
revision, metadata, content or schema pin conflict. A new ID must be fresh at the
database clock and have a strictly newer generation instant; equal/older generations
are rejected. Scope serializes concurrent writers, with freshness rechecked at commit.

Failed fetching, validation or transactions retain the prior view and all history.
An empty complete refresh advances current to empty membership; removed resources
cease current membership while their evidence remains. Explicit retirement is
preserved. No selection, eligibility or execution is inferred from stored references.
Identity fields cannot contain NUL; labels remain opaque source evidence. Source
pages are canonical UTF-8 bytes, with relational keys stored separately. Fingerprints
are versioned; timestamps use UTC microsecond database precision.

`-stored` emits the current page array without contacting Perfeng or requiring its
token, endpoint or producer version. It is mutually exclusive with `-publish`.
Current means last published: expired evidence remains inspectable and its original
expiry is retained. A missing scope returns an error, never an empty success.
Database credentials confer trusted service authority; scope filters and composite
keys are not tenant authentication. Deployment owns role restrictions and clocks.

Cancellation/deadlines propagate, lock waits are bounded to five seconds and each
database operation to fifteen seconds. Errors exclude SQL details and credentials.
Commit can succeed before readback/stdout fails: inspect stored current and retry
the same source snapshot to resolve ambiguity. Do not assume an output failure
rolled back publication. Rollback disables `-publish` and retains additive tables;
there is no destructive down migration or automatic startup DDL.

## Verification

Run `go test -vet=off ./internal/performance/... ./internal/architecture`
and `make validate`. Executable metadata/exit smoke tests cover the inspection
command. HTTP tests exercise loopback authentication, opaque cursors, bounded
restart, redirects, cancellation, stale/wrong scope and invalid/oversized responses.
Run `make db-validate` with a disposable loopback PostgreSQL 17.11 server through
`ARGUS_TEST_POSTGRES_URL`. Tests cover full HTTP-to-publication flow, retained views
on failure, replay/conflict, concurrent writers, rollback, empty/retired membership
and immutable resource guards, as well as the complete migration/upgrade chain.
See [ADR-0052](../decisions/0052-pin-perfeng-catalog-validation.md) and
[ADR-0053](../decisions/0053-fetch-authorized-catalogs-for-inspection.md) and
[ADR-0054](../decisions/0054-publish-immutable-performance-catalogs-atomically.md).
