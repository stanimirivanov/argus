# Perfeng request contract boundary

## TL;DR

- M09-PR7a pins messages; PR7b compiles explicitly approved request evidence.
- Decode/encode checks consistency, never authentication or execution authority.
- PR7 completes through reviewed compilation, durable intent/outbox and operator CLI.

## Pinned interchange

The [performance adapter](../../internal/performance/adapters/perfeng/integration.go)
adopts performance-integration/v1, API 1.0.0, from
[contracts commit 041273947703542f7fe22eab838c176df5517974](https://github.com/stanimirivanov/perfeng-contracts/tree/041273947703542f7fe22eab838c176df5517974),
bundle 0.24.0, matching the merged provider. The schema, OpenAPI and three published
exchanges live under its `snapshot/integration/` directory with byte checksum tests.
The parent snapshot Apache-2.0 license covers the copied artifacts. These are
externally owned pins, not Argus-generated Effect schemas.

`DecodeIntegration` and `EncodeIntegration` handle request, accepted, rejected,
lifecycle and cancellation messages. They enforce the 64 KiB bound, unique keys,
valid Unicode, closed shapes, formats and per-message identity/time semantics.
Only embedded schema resources resolve. Decode returns owned values; neither
operation performs I/O, consults a clock or mutates caller data. Errors expose no
provider diagnostics; unsupported generations also classify as `ErrIntegrationVersion`.

Request and attempt identities differ. Request/cancellation event source equals
the request issuer. Deadline follows creation, events cannot predate creation,
and acceptance precedes deadline. Late cancellation and lifecycle evidence remain
valid. JSON number revisions preserve integer-valued exponent syntax and the
published safe-integer bound; timestamps preserve offsets and microsecond precision.

## Authority and remaining work

Schema validity does not prove producer identity, principal scope, review authority,
freshness, reference association, or approval. Typed values remain transport claims.
Runtime consumers must verify expected provider/principal independently, retain
immutable context, reject conflicting deliveries and prevent terminal regression.
Successful completion does not establish a passing performance verdict.

Selection is a recommendation. Request compilation must require explicit trusted
owner approval and current source/catalog/policy evidence. Missing service,
capability or deployment evidence must never be inferred from display/local IDs.
See [the PR7 delivery plan](../development/plans/m09-pr7-performance-requests.md).

## Reviewed compilation

`selectionio.Compile` is a pure driving-boundary compiler over inspected evidence,
original policy bytes, a typed `RequestReview` and independently trusted
`RequestAuthority`. Domain selection is rerun at the supplied current clock.
The approved selection must match every current pin and decision; only its original
evaluation instant is retained. Approval cannot predate that evaluation or extend
past selection expiry. Each request deadline must be live and within approval expiry.

The composition root must authenticate the reviewer, obtain the approved SHA-256
of `json.Marshal(RequestReview)` independently, and supply the authorized principal,
request issuer and exact owner-route versions. Hashing a submitted review and
passing its own claims as authority is invalid integration. These internal Go
values introduce no public review-file format or credential mechanism. The future
operator entry point must establish this boundary before enabling creation.

The review supplies complete shared request messages, including explicit service,
capability and evidence references, source/image pins and frozen request/attempt/event
identities. Compilation validates the pinned shared schema and exact selected
deployment, service identity, workload, profile, environment, policy and baseline.
Provider capability and evidence associations are explicit owner-reviewed claims;
they cannot be independently discovered from the present catalog. Perfeng retains
its own admission authority. Missing references never become invented resources.

Every candidate needs exactly one binding; omitted decisions remain in retained
selection evidence, and unresolved selections fail closed. Identical payloads for
identical target contexts combine all mapping/capability/reason records. Different
identities for the same target, or reused event/request/attempt identities for
different payloads, reject the whole batch. Input order determines stable output
order. Recompiling the same review while live preserves bytes and identities.

`Compilation.Snapshot` returns independent copies of original policy bytes,
canonical selection/review bytes, validated payloads, payload digests and reasons.
No partial snapshot is returned on failure. This is evidence for PR7c persistence;
it does not insert rows, enqueue work or send a request. Rollback is a code revert;
there is no activation flag, migration or production behavior change.

Run focused adapter/architecture tests and `make validate` for the contract and
compiler above. The durable evidence foundation below also needs the SQL gate.

## Durable evidence foundation

PR7c1 adds `Compilation.Intent`: an inward value with a versioned fingerprint and
owned copies of retained policy/selection/review evidence, exact child payloads,
digests, identities, timestamps and reasons. The requestintent application package
validates inward integrity; it neither parses provider DTOs nor grants authority.
PostgreSQL and future dispatch consumers can use it without importing selectionio.

The additive schema provides scoped intent and pending-dispatch rows, identity
uniqueness, exact child counts/ordinals and a parent/current-catalog relationship.
Deferred guards reject incomplete assembly, future or expired work, and a changed
or expired catalog. Rows cannot be changed or deleted; children cannot be appended
after the creating transaction commits. The pending position is an internal keyset
cursor, not a lease, acknowledgement or durable snapshot.

`PerformanceStore.CreateRequest` atomically retains an intent and every child,
sharing the catalog publication scope lock. Exact review/fingerprint replay is
read-only even after catalog replacement or expiry. Changed evidence or reused
child identities return `requestintent.ErrConflict`; non-live children return
`ErrExpired`. New creation requires the current live catalog at commit. A lost
commit response has an uncertain outcome: retry the exact immutable intent.

`GetRequest` reads owned evidence by review digest in the independently authorized
provider/principal scope; wrong-scope and missing reads both return `ErrNotFound`.
`PendingRequests` reads owned child evidence in increasing database-local cursor
order with limits 1..100 and a nonnegative `after` cursor. Expired work remains
visible for recovery. Concurrent writers can commit out of cursor allocation order;
restart from zero rather than treating pages as a durable snapshot or lease.
Invalid input returns `ErrInvalid`; cancellation/deadlines propagate, and other
storage failures return redacted `ErrUnavailable`. Reads and writes are bounded by
the runtime operation timeout. These methods do not authenticate embedded claims.

PR7d1 adds the read-only operator entry point below; PR7d2 adds independently
authorized reviewed creation. PR8 owns dispatch progress separately from immutable
work. No request-creation CLI activation or network dispatch is introduced here.

Apply migration `20261025093826_add_performance_request_intents.sql` before new
writers. No backfill or existing-table rewrite is required. Rollback disables new
creation while retaining immutable rows; never delete pending evidence to roll back.
Run `make validate` and `make db-validate` on disposable PostgreSQL 17.11. Direct SQL
tests prove schema guards independently; adapter tests verify concurrent replay,
atomic failure, complete evidence recovery after reopening, and scoped pagination.

## Operator evidence inspection

`go run ./cmd/performance-requests` is opt-in. Configure `ARGUS_DATABASE_URL` through
your secret manager/environment with read-only access to the request-intent and
pending-dispatch tables. No provider URL or token is needed; the command neither
runs migrations nor calls the provider.

```sh
go run ./cmd/performance-requests -pending -after 0 -limit 25 \
  -issuer https://provider.example/identity \
  -principal-issuer https://auth.example/tenant -principal-id argus-service
```

Replace `-pending -after 0 -limit 25` with `-review-sha256 <lowercase-64-hex-digest>`
to inspect one complete intent. Scope arguments must come from trusted operator/CI
configuration independently of submitted evidence; they do not authenticate the
caller. Database credentials and scope authorization remain deployment duties.
The command has no creation, dispatch, acknowledgement or cancellation operation.

Success emits one local diagnostic JSON document with the exact `scope`, and either
`fingerprint` plus
`intent` or a `pending` array plus `nextAfter`. Retained evidence uses the version-one
inward projection's field names; binary evidence and frozen payloads are base64 JSON
strings. Treat output as sensitive reviewed evidence. Like catalog publication
diagnostics, its shape follows this executable's internal projection; it is not a
portable Argus data contract or provider interchange. An independently consumed
data contract would require Effect Schema and compatibility fixtures under ADR-0003.
An empty page has `pending: []` and preserves the supplied cursor. A nonempty page's
`nextAfter` is its last position; restart at zero across concurrent writes. Expired
work remains visible, and inspection says nothing about execution eligibility.

Usage errors exit 2 before opening storage. Storage, cancellation and output failures
exit 1 with redacted database diagnostics; success exits 0. No success JSON is written
after a failed read or observed cancellation. Output errors may leave partial bytes:
consume only after exit 0. Ctrl+C/SIGTERM cancels reads. `--help`/`--version` require no
credentials. Run `make validate` and `make db-validate`; tests include a connection
with `default_transaction_read_only=on`.
