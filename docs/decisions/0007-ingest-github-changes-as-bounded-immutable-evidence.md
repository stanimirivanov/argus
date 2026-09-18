# ADR-0007: Ingest GitHub changes as bounded immutable evidence

- Status: Accepted
- Date: 2026-09-18
- Milestone: M04 - Change impact
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Authenticate GitHub webhooks over the exact raw body before parsing them.
- Bind each delivery ID to its signed-body digest and immutable base/head pair.
- Read pull-request state before and after file pagination; reject movement as
  stale rather than publishing mixed evidence.
- Retain at most 1,000 files, 64 KiB of patch text per file, and 2 MiB of patch
  text overall, with explicit truncation states.
- Persist the delivery and normalized `ChangeSet` atomically; exact retries are
  no-ops and conflicting identity reuse is rejected.

## Context

M04 needs a trustworthy input for deterministic impact analysis. A webhook is
only a notification: a pull request may advance while Argus paginates files,
the provider may omit binary or oversized patches, and delivery retries may be
simultaneous. Treating the current pull request response as though it belonged
to the signed event can silently associate files with the wrong revision.

GitHub is the first SCM adapter, but the application and domain must not adopt
GitHub payload types. Ingestion also needs bounded memory and storage without
allowing omitted data to look like proof of no impact. General user and
workload authorization remains M10 scope; webhook HMAC is the authentication
boundary for this provider-owned endpoint.

## Decision

Argus verifies `X-Hub-Signature-256` with HMAC-SHA-256 over the exact request
body and a configured secret before JSON parsing. It accepts the supported
`pull_request` actions only and normalizes repository identity, delivery ID,
pull-request number, update time, and full Git SHA-1 revisions into an
application-owned delivery value. The HTTP adapter accepts at most 25 MiB.

The GitHub resolver fetches current pull-request metadata, requires its
repository ID and base/head revisions to match the signed delivery, paginates
changed files, and fetches metadata again. Any base, head, or changed-file
count movement returns a stale outcome. Provider calls occur outside database
transactions. Argus retains at most 1,000 files, 64 KiB of UTF-8 patch text per
file, and 2 MiB in total. Missing, truncated, and budget-exhausted patch text
and a truncated file list are represented explicitly in `ChangeSet` v1.

PostgreSQL claims `(provider, delivery ID)` in the same transaction that stores
the canonical change set and file rows. It stores both the signed-body SHA-256
and normalized-content SHA-256. An exact retry returns the stored result; a
different body or normalized result at the same delivery identity is a
conflict. The application checks for an existing delivery before provider I/O,
while the database uniqueness constraint resolves concurrent first attempts.

## Alternatives considered

### Trust payload file data

- Benefits: One request and no REST token.
- Costs and risks: Pull-request webhook payloads do not provide the bounded,
  complete per-file evidence required by impact analysis.
- Reason not selected: The result would not satisfy the `ChangeSet` contract.

### Fetch files without a consistency fence

- Benefits: Fewer API calls.
- Costs and risks: Pagination can combine state from different pull-request
  heads and falsely label it with the webhook revisions.
- Reason not selected: Immutable provenance is more important than one saved
  metadata request.

### Store unbounded provider responses

- Benefits: Maximum available provider detail.
- Costs and risks: Unbounded memory, database growth, and transport payloads;
  GitHub still may omit patches, so “complete” would remain misleading.
- Reason not selected: Explicit bounded evidence gives consumers safer
  semantics and predictable resources.

### Deduplicate only in process memory

- Benefits: No migration or database transaction.
- Costs and risks: Retries after restart and concurrent replicas can repeat
  provider work or produce divergent results.
- Reason not selected: Delivery identity is a durability boundary.

## Consequences

### Positive

- Every normalized file list is attributable to verified immutable revisions.
- Retries are durable across restarts and safe across concurrent replicas.
- Downstream impact policy can distinguish absence from unavailable or partial
  evidence and choose a conservative fallback.
- GitHub details remain outside the change domain and ingestion application.

### Negative

- A first delivery requires two pull-request metadata reads plus paginated file
  reads and a read-scoped GitHub token.
- Pull requests beyond the bounds require later conservative processing rather
  than a complete file-level answer from this record alone.
- PostgreSQL storage grows with immutable deliveries and retained patch text.

### Neutral or follow-up

- Semantic OpenAPI comparison and affected-capability mapping are later M04
  slices that consume this evidence.
- Retention, replay administration, GitHub App installation-token acquisition,
  and non-GitHub providers remain separate decisions.
- A future contract major may add other revision algorithms or provider event
  types without weakening v1 semantics.

## Compatibility and migration

The public addition is `argus.dev/change-set/v1`, authored in Effect Schema and
exported as generated JSON Schema. The database migration only creates new
tables and indexes; it performs no backfill, scan, or rewrite. Older binaries
ignore the tables. Application rollback leaves evidence intact, and migration
rollback uses a forward repair rather than dropping accepted observations.

## Security and operations

Webhook secrets, GitHub tokens, and database URLs come from environment-backed
secret configuration and are never returned or logged. Signature comparison is
constant-time. The GitHub token needs read-only pull-request and repository
metadata access. HTTPS is required except for loopback test servers. HTTP and
provider response bodies, file counts, and patch bytes are bounded.

Migrations remain an explicit privileged command. The runtime role needs
select/insert access to the new tables and sequence plus the existing limited
repository-coordinate update. Operational retries should redeliver the same
GitHub delivery; callers must not invent a new identity to bypass a conflict.

## Validation

The decision is verified by shared Effect/JSON-Schema fixtures, Go domain and
contract tests, the GitHub documented signature vector, signature-before-parse
tests, REST header/pagination and head-movement tests, HTTP error-path tests,
application retry and mismatch tests, migration-chain checks, PostgreSQL
round-trip/restart/conflict/concurrency tests, the architecture import test,
`make validate`, and `make db-validate` against PostgreSQL 17.11.
