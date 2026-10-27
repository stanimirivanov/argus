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
values introduce no portable review contract or credential mechanism. The operator
creation path below delegates authenticated approval issuance to trusted operator/CI
configuration, independently of submitted files.

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

PR7d1 adds inspection and PR7d2 adds independently authorized reviewed creation below.
PR8 owns dispatch progress separately from immutable work; creation never contacts
the provider or claims processing work.

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
These inspection modes never create, dispatch, acknowledge or cancel work.

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

## Reviewed operator creation

The explicit leading `-create` option enables creation. The authenticated owner/CI
approval process must independently authorize the reviewer, principal, request issuer,
exact owner-route references and canonical review SHA-256. It supplies a protected
file via `ARGUS_PERFORMANCE_REQUEST_APPROVAL_FILE`, outside untrusted checkout/input
control, mounted read-only or protected by deployment ACLs. File bytes do not prove
authentication; trusted origin, permissions and approval issuance are host duties.
There is no authority flag, signature verifier or fallback to review claims.

The local approval artifact is `json.Marshal(selectionio.RequestApproval)`: `Authority`
contains `RequestAuthority` and `Expected` contains independently trusted provider,
principal and producer expectations. The submitted review is canonical
`json.Marshal(selectionio.RequestReview)`. These are internal operator artifacts,
not portable contracts. The approval hash pins the marshaled typed review, not a
hash computed by this command to manufacture approval. Inputs accept surrounding
whitespace but reject noncanonical fields/order, duplicate keys, aliases, invalid
Unicode and files larger than 16 MiB. Original policy bytes retain their exact hash.

Configure `ARGUS_DATABASE_URL` for the existing scoped evidence reads and request
inserts, with the PR7c1 migration already applied, then invoke:

```sh
go run ./cmd/performance-requests -create -delivery-id delivery-42 \
  -policy reviewed-policy.json -review reviewed-request.json
```

Provider settings come only from the approval file. Creation reloads the exact GitHub
delivery and current scoped catalog, rechecks provenance and approval through the
existing compiler at actual time after reads, then atomically retains intent and all
children. No identities are regenerated. Success emits local diagnostic JSON with
`created`, `fingerprint` and `reviewSha256`; repeated live creation returns
`created: false`. Invalid/unapproved, stale, conflicting or cancelled work emits no
success document. Exit codes match inspection.

A commit response, cancellation or stdout failure can leave outcome uncertain. Retain
the reviewed identities and inspect using the approved digest with `-review-sha256`
and the independently authorized scope. Creation retries revalidate current evidence;
after expiry/catalog changes, use inspection to recover retained history. Do not
invent fresh identities to retry. Rollback disables the new option/configuration
and retains all evidence; dispatch remains PR8. Testdata is synthetic and expired,
not production approval. Run `make validate` and `make db-validate`.

## Provider admission transport

`perfeng.IntegrationHTTP` is the opt-in PR8a embedding boundary. Configure the exact
`/integration/v1/requests` collection endpoint, bearer credential, expected provider
issuer and principal independently of submitted messages. HTTPS is required except
literal loopback HTTP for local testing. No command or worker is enabled by this slice;
see the [PR8 delivery plan](../development/plans/m09-pr8-performance-dispatch.md).

`Submit` validates the frozen requested message and sends its original bytes once.
`Lookup` takes that original message and escapes its issuer-scoped request ID for
readback. Both require a matching configured principal and return only schema-valid
messages with the exact original context and configured provider/run issuer.
Submission expects accepted/202 or rejected/422; lookup expects lifecycle/200 or
rejected/422. The current provider uses 422 for durable rejection although the pinned
OpenAPI omits that status; pinned artifacts remain unchanged. Rejection is evidence,
not execution success, and a lifecycle snapshot is not ordered/retained by this adapter.

Each exchange has a ten-second bound and a 64 KiB response limit. Client settings
are cloned, cookies and redirects are disabled, and errors omit network/remote-body
diagnostics. HTTP failures expose status only; cancellation/deadlines propagate.
There are no automatic retries. A failed submission or a lookup 404 cannot establish
that admission never committed. Retain identities and payloads for the later fenced
worker's recovery; this transport does not authenticate approval, enforce dispatch
eligibility or persist outcomes. Run HTTP/contract tests and `make validate`.

## Durable dispatch leases

PR8b1 adds `PerformanceStore.ClaimRequest(scope, lease)` and
`ReleaseRequest(scope, claim, delay)` over independently authorized scope.
Acquisition skips locks and returns owned evidence, token, fence and database-clock
expiry; no candidate/wrong scope returns `ErrNotFound`. Leases accept 1s..5m and retry
delays 0..1h in whole microseconds; invalid inputs return `ErrInvalid`. Stale, expired
or already released owners get `ErrConflict`. Cancellation propagates; diagnostics
are redacted. Claims can expire in transit and never authorize submission or
acknowledge completion. Uncertain commits must not trigger sending; preserve frozen
identities. Expired requests remain claimable only for lookup recovery.

Apply `20261026092008_add_performance_dispatch_claims.sql` before callers; no worker
is enabled. PR8b2 adds outcomes, PR8c the worker. [ADR-0059](../decisions/0059-lease-performance-dispatch-with-monotonic-fencing.md)
documents privileges, migration locks/headroom, retry uncertainty, rollback and tests.

## Validated dispatch outcomes

PR8b2a adds `perfeng.DispatchOutcome(frozen, reply, provider)` to convert schema-valid
accepted/rejected admission responses or lifecycle lookup snapshots. The original
must be requested.v1; every context field, provider source and run issuer must match.
Pass independently authenticated provider configuration and provenance. Invalid
inputs return zero evidence and a redacted `ErrIntegration`.

The owned `requestintent.Outcome` retains identities, source/event ID, kind and
canonical provider message bytes with a versioned fingerprint. It does not preserve
HTTP formatting, order run lifecycle, grant approval, persist evidence or enable a
worker. [ADR-0060](../decisions/0060-validate-immutable-performance-dispatch-outcomes.md)
defines the projection and dependent PR8b2b/PR8c responsibilities. Run focused
requestintent/Perfeng tests and `make validate`.

## Durable dispatch settlement

PR8b2b adds `PerformanceStore.SettleRequest(scope, claim, outcome)` and
`RequestOutcome(scope, position)`. Settlement requires live database ownership;
exact winning-owner replay returns false even after expiry. Changed evidence,
scope/child or ownership conflicts; malformed proof/provider evidence is invalid.
Scoped reads hide absence/wrong scope and redact corrupt evidence. Settled work
cannot be claimed or released, while pending inspection preserves its history.
These APIs perform no provider calls and enable no worker. [ADR-0061](../decisions/0061-settle-performance-dispatch-under-live-fences.md)
records migration locks, required privileges, uncertainty and rollback. Apply the
outcome migration before callers; verify with `make validate` and `make db-validate`.

## Lookup-only admission recovery

PR8c1 adds `requestintent.Recover` over a store-issued claim and independently
authorized scope. Its consumer-owned ports use existing scoped outcome reads and
fenced settlement. [ADR-0062](../decisions/0062-recover-performance-admission-through-scoped-readback.md)
records the boundary. `perfeng.IntegrationHTTP.LookupOutcome` supplies the configured
provider port: it checks provider/principal scope, child integrity, request/attempt/
event identities and timestamps against the frozen bytes before any network access.
The existing authenticated HTTP boundary validates the complete response context.

Recovery first returns already retained correlated evidence without contacting the
provider. Otherwise it performs one bounded lookup of the original request and
settles the owned result under the supplied fence. Request deadlines do not prevent
readback; only the database can authorize a live settlement. On settlement failure,
one scoped read can recover a lost acknowledgement or another owner's retained
disposition. If that read also fails, the original settlement error is preserved.
Cancellation always returns no success evidence, even if a write committed; inspect
or recover again with the same identities and independently authorized scope.

Lookup absence (including 404), provider/storage failures and stale ownership leave
unresolved evidence intact. The operation never submits, generates identities,
acquires/releases leases, retries provider calls or schedules work. It cannot
resolve an unsubmitted request absent at the provider. No command or worker is
activated, and ordered lifecycle remains PR9. PR8c2 owns initial-send/crash policy
and opt-in composition. Rollback removes callers without altering retained data.
Run focused recovery/Perfeng tests and `make validate`; there are no schema/query
changes or new database privileges in this slice.

## Issuer-filtered acquisition

PR8c2a1 adds `PerformanceStore.ClaimRequestForIssuer(scope, issuer, lease)`. Supply
independently authorized provider/principal scope and request issuer. Both frozen
request and attempt issuers must match before acquiring ownership; unrelated and
mixed-issuer work retains first-acquisition eligibility. Empty/whitespace, invalid
UTF-8, NUL-containing or over-512-rune issuers reject before storage. Existing scope
and lease bounds apply; no eligible work/wrong scope returns `ErrNotFound`.

The method shares database-clock leases, monotonic fences, scoped ownership,
outcome exclusion, redaction and uncertain-commit behavior with `ClaimRequest`.
Legacy `ClaimRequest` remains unfiltered. No schema, wire, index or privilege changes
are needed. A claim still grants no approval or submission authority. This slice
activates no command or worker; PR8c2a2/PR8c2b retain dispatch/runtime composition.
Rollback disables callers and preserves evidence/fences. See
[ADR-0063](../decisions/0063-acquire-performance-work-by-authorized-request-issuer.md).
Run focused issuer-claim tests, `make validate` and `make db-validate`.

## One-shot dispatch

PR8c2a2 adds `requestintent.Dispatcher.One(ctx)` over independently authorized,
immutable provider/principal scope, request issuer, lease and trusted clock. Only
previously approved frozen work may enter storage. Offline provider preflight
precedes issuer-filtered acquisition; a lease never grants approval. Fence one may
submit exact bytes once while request/lease remain live, with a context bounded by
both. The configured Perfeng bridge validates correlation and returns owned
accepted/rejected evidence for fenced settlement and uncertain-write readback.

Reacquisitions and expired work use lookup-only recovery. Errors preserve leases
until expiry; there is no automatic release, retry or identity generation. A crash
before POST or lost acquisition acknowledgement can strand absent work; 404 never
authorizes replay. Provider admission and database fences remain authoritative
despite context bounds. No schema/wire change or worker activation occurs here;
PR8c2b retains runtime composition. See [ADR-0064](../decisions/0064-submit-performance-work-only-from-first-acquisitions.md).
Run application/HTTP and database dispatch tests, `make validate`, `make db-validate`.

## Opt-in control-plane worker

PR8c2b enables dispatch only when `ARGUS_PERFORMANCE_DISPATCH_ENABLED=true`.
Empty/`false` leaves existing server behavior unchanged and ignores other worker
settings. Invalid opt-in or enabled configuration fails before database startup.
Supply these environment settings through the trusted operator/secret process;
no endpoint, token or authority is taken from retained messages.

| `ARGUS_PERFORMANCE_DISPATCH_` suffix | Enabled configuration |
|:--|:--|
| `URL` | Required `/integration/v1/requests` endpoint; HTTPS, or literal loopback IP for local HTTP |
| `TOKEN` | Required bearer token; never pass it as a command argument |
| `PROVIDER` | Required independently authorized provider issuer URI |
| `PRINCIPAL_ISSUER`, `PRINCIPAL_ID` | Required provider-authenticated principal binding |
| `REQUEST_ISSUER` | Required independently authorized request/attempt issuer URI |
| `LEASE` | Default `30s`; `1s..5m` in whole microseconds |
| `POLL_INTERVAL` | Default `5s`; `1s..1m` in whole microseconds |

Run the existing `control-plane` command after explicit migrations; startup does
not migrate or add grants. Use existing runtime privileges, never a migrator role.
One binding runs per process. Calls are serial: immediately, then at most once per
tick (slow calls coalesce ticks). Idle/error results wait for a tick, without
automatic POST replay or release. Prefer a lease exceeding the HTTP ten-second
bound. Other issuers retain eligibility; replicas coordinate through fenced claims.

Shutdown cancels polling/provider I/O immediately, while HTTP/DBOS retain their
existing drain lifetime. Cleanup joins the worker before closing database resources.
Logs identify the worker and retained outcome kind or unresolved operation, without
raw errors, credentials, bodies or identities. Inspect scoped pending/outcome
evidence to investigate; lookup absence cannot prove an uncertain POST never committed.
Disable the flag and restart to roll back without deleting evidence/fences. Lifecycle
ordering, cancellation/reruns and M10 recovery remain separate. See
[ADR-0065](../decisions/0065-opt-in-to-configured-performance-workers.md).
Run worker/configuration/shutdown tests, `make validate` and `make db-validate`.

## Ordered lifecycle observations

PR9a adds `perfeng.LifecycleObservation` and `requestintent.ProjectLifecycle`.
The adapter validates the pinned message and complete frozen context before
constructing owned evidence; callers must authenticate the configured provider
and principal independently. The projection accepts 1..1000 observations from one
exact frozen request/run binding, sorts a copy by revision, and returns zero on
any conflict. Equal state/revision snapshots can repeat under new envelopes;
reused event IDs require identical canonical evidence. Contradictory historical
revisions reject the whole projection rather than silently disappearing.

Skipped states and repeated nonterminal states at increasing revisions are valid.
Terminal snapshots are immutable, including their revision. Arrival and event
time do not establish ordering; `Late` compares provider occurrence with the
frozen deadline and never discards valid evidence. COMPLETED does not mean PASS.
This callable foundation changes no wire, schema, worker or deployment behavior.
Durable storage and authenticated runtime consumption follow the
[PR9 delivery plan](../development/plans/m09-pr9-performance-lifecycle.md).
Run focused requestintent/Perfeng tests, `make architecture-check`, and `make validate`.

PR9b1 adds `BindLifecycleAdmission`: it revalidates canonical retained settlement
and frozen request bytes, yielding an owned run binding and initial snapshot for
lifecycle admission. The additive lifecycle history table guards scope/admission,
event reuse, run identity, revision order, terminal immutability and 1000 rows per
request. Its recording instant is database-owned; late provider evidence remains
valid. These are foundations for PR9b2 atomic append/scoped projection, not a new
writer or polling path. Apply explicit migrations first; retain the additive schema
on application rollback. See [ADR-0067](../decisions/0067-bind-lifecycle-history-to-retained-admission.md)
for trust, privileges and lock behavior. Run `make validate` and `make db-validate`;
busy-host verification may run on existing CI with results reported separately.
