# ADR-0054: Publish immutable performance catalogs atomically

- Status: Proposed
- Date: 2026-10-23
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Reuse the pinned validator and authenticated fetcher before opening a transaction.
- Persist immutable source evidence, relational identities and one scoped current pointer atomically.
- Exact replay never rewinds current; new snapshots must be fresh and strictly newer.
- Apply the additive migration before enabling the explicit `-publish` mode.
- Stored evidence remains inspectable after expiry and conveys no execution authority.

## Context

Validated operator output is not durable publication. A refresh must never expose
partial membership, overwrite conflicting evidence, or replace a newer view with
an older concurrent result. The existing PostgreSQL runtime already owns bounded
transactions and capability-scoped store views.

## Decision

Add the inward `catalogsync` evidence value and store port, implemented by
`postgres.PerformanceStore`. The command fetches and revalidates complete pages
before database publication. No external call occurs inside the transaction.
No dependency or provider private model is introduced.

Scope is the exact provider issuer, principal issuer and principal ID tuple.
Within scope, a source snapshot ID binds all evidence, including its revision,
source pages, resource projection and pinned schema hash. A differing retry is a
conflict. A matching retry returns the original durable view without repointing
current, even when that snapshot is historical or expired. Different source IDs
require strictly increasing generation instants; equal instants are ambiguous
and rejected. Resource identity is snapshot, kind, issuer and resource ID.

The application creates an immutable canonical copy. Fingerprints hash the
versioned `argus.performance-catalog/v1` JSON encoding of inward `Evidence` field
names and values. Source object keys/whitespace and resource ordering are
normalized. Timestamps are normalized to UTC microsecond precision for PostgreSQL;
source page timestamps retain their original evidence. Evolving this encoding
requires an explicit version/migration rather than silently changing fingerprints.
Source pages are opaque UTF-8 bytes in BYTEA, retaining source strings PostgreSQL
JSONB cannot represent (such as an escaped NUL in a label). Relational identity
strings reject NUL; resource lifecycle is constrained separately in SQL.

Publication takes a transaction advisory lock keyed by scope, checks identity and
database-clock freshness, inserts the header and all resources, and advances the
scoped pointer. Deferred triggers recheck freshness, complete resource counts and
monotonic generation at commit. Composite foreign keys bind the pointer to its
scope. Updates/deletes of evidence and deletion of the current pointer are denied.
An XID8 assembly transaction marker prevents appending resources after publication;
a deferred header trigger verifies completeness even without a pointer update.
No per-resource deferred count scan is introduced.

Historical reads capture one immutable snapshot UUID, then reconstruct and verify
its complete fingerprint. Current means last published, not currently eligible.
Empty refreshes publish empty membership; removed identities remain in history.
Errors are bounded capability errors, preserving cancellation and deadlines without
returning SQL messages, provider bodies or credentials. A commit may succeed before
readback/output fails; retry the same source ID to resolve an ambiguous result.

## Consequences and compatibility

Default inspection keeps its existing page-array output. `-publish` adds an opt-in
`created`/`pages` envelope; `-stored` reads the scoped current page array without a
provider credential or connection. Both database modes need `ARGUS_DATABASE_URL`.
There is no background scheduler, selection, launch, baseline approval or outbox
event in this slice. Callers must recheck freshness at future use boundaries.

Apply the new forward-only migration before using publication. Runtime roles need
SELECT/INSERT on snapshot/resources and SELECT/INSERT/UPDATE on current, with schema
USAGE and function execution; they must not own the schema or have DDL/TRUNCATE
authority. Credentials remain trusted service authority: scoped queries and keys
prevent accidental association, and do not constitute tenant authentication/RLS.
Deployment owns secrets, roles, trusted clocks, backup and retention. Rollback
disables publication or reverts the application while retaining the additive tables
and evidence. No destructive down migration or automatic startup DDL is provided.

## Verification

`make validate` covers core/import/CLI behavior and architecture boundaries.
`make db-validate` covers empty/repeated migrations, released-schema upgrade,
atomic publication/readback, HTTP-to-publication flow, replay, conflict, freshness,
empty membership, retirement, concurrent writers and rollback/immutability guards.
