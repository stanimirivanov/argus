# ADR-0018: Store one immutable outcome per adaptation review

- Status: Accepted
- Date: 2026-09-27
- Milestone: M06 - Validated adaptation
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Persist one immutable terminal `ReviewOutcome` per adaptation `reviewId`.
- Verify the existing v1 public outcome ID before database access and preserve
  its already-published derivation algorithm.
- Treat a later observation of identical terminal state as an exact retry;
  observation time does not create another outcome.
- Store the outcome and every reviewer patch atomically in PostgreSQL.
- Reject changed disposition, reason, provenance, revision, or patches for an
  already claimed review rather than overwriting evidence.
- Retrieval is by explicit outcome ID under repeatable read; no latest lookup,
  provider call, or learning-policy update occurs.

## Context

ADR-0017 defines portable terminal review evidence but deliberately leaves
durability and learning separate. A CI artifact alone cannot support reliable
restart behavior, exact-retry ingestion, or later aggregate evaluation. The
first durable boundary must also decide whether recapturing the same closed PR
at a later time is a duplicate and whether two different reason codes may be
stored for one review.

Review outcomes are intended as immutable evidence. Allowing multiple terminal
records for one review would make acceptance rates and correction counts
ambiguous. Conversely, treating `observedAt` as semantic would turn harmless
polling retries into conflicts even when GitHub state and the human reason are
identical.

## Decision

### Verify semantic identity before persistence

The outcome ID preserves ADR-0017's published v1 derivation across repair-chain
IDs, decision and reason, final revision, terminal provider time, and canonical
reviewer edits. `observedAt` remains excluded because it describes when
identical terminal evidence was read, not a different review disposition.
Reason notes and textual patches cannot contain NUL bytes, preserving the
unambiguous delimiter assumptions of the v1 algorithm.

Contract import recomputes this identity. A merely hash-shaped but incorrect
ID is invalid and cannot open the database adapter. Repository, pull-request,
generated-revision, and other complete provenance fields are additionally
bound by the private persistence fingerprint without changing the public v1
contract.

### Claim one outcome per review

PostgreSQL uniquely constrains both `outcomeId` and `reviewId`. The store claims
the review identity and compares a private canonical fingerprint that includes
all semantic fields but excludes observation time. An exact retry returns
`created: false`. Different evidence for an existing review returns
`ErrOutcomeConflict`; no row is updated or replaced.

The first successful observation time remains durable. A corrected reason or
provider discrepancy is handled as an explicit conflict requiring operator
investigation, not an in-place evidence edit.

### Store and reconstruct complete evidence atomically

The outcome header and normalized reviewer-edit rows commit in one short
transaction after all provider access has already completed. Database
constraints reinforce hashes, decision/reason combinations, revisions,
timestamps, paths, change counts, and patch bounds.

Reads name one explicit outcome ID, use a read-only repeatable-read transaction,
load edits in canonical path order, and validate the reconstructed domain value
before returning the versioned contract.

## Alternatives considered

### Store every observation as a separate event

- Benefits: Preserves polling history and permits later reason correction.
- Costs and risks: Duplicates one terminal review, makes outcome metrics
  ambiguous, and mixes observation telemetry with semantic evidence.
- Reason not selected: Operational polling history can be added separately if
  needed; the evidence corpus needs one authoritative terminal record.

### Update the existing review row on retry

- Benefits: Could revise a mistaken reason or later provider state.
- Costs and risks: Destroys auditability and lets automation silently rewrite
  evidence used for evaluation.
- Reason not selected: Corrections require an explicit future supersession
  model, not mutable rows.

### Store only the portable JSON document

- Benefits: Smaller schema and simple round-trip behavior.
- Costs and risks: Database constraints cannot protect review identity,
  decision/reason meaning, revision shape, or individual patch bounds.
- Reason not selected: The outcome and reviewer edits have stable relational
  identity and will be queried for later evaluation.

## Consequences

### Positive

- Captured outcomes survive process restart and concurrent retries.
- Later evaluation receives one unambiguous record per adaptation review.
- Retry behavior is stable even when the same closed review is observed later.
- Relational constraints and domain reconstruction detect corrupt evidence.

### Negative

- A mistaken reviewer reason cannot be edited in place; it currently requires
  investigation and a future explicit correction mechanism.
- PostgreSQL storage contains textual reviewer patches and therefore needs the
  same access controls and retention review as other source-derived evidence.
- Contributors changing persisted identity semantics must preserve or migrate
  the private fingerprint and public outcome-ID behavior deliberately.

### Neutral or follow-up

- Aggregate outcome queries, dashboards, retention, and governed policy
  learning remain future work.
- Unsuccessful validation evidence remains outside this schema.
- Review patch storage does not replace Git history or artifact retention.

## Compatibility and migration

The additive migration creates empty outcome and reviewer-edit tables. It does
not scan, rewrite, or backfill existing tables. Older application versions
ignore the new tables, and rollback leaves unused evidence intact.

The existing public outcome-ID algorithm is preserved by a golden test. The private retry
fingerprint is separately golden-tested. Changing either requires an explicit
compatibility plan because existing identities and exact-retry behavior are
durable data meaning.

## Security and operations

Only the local administrative CLI is exposed in this slice. It validates and
bounds the complete document before opening the database. Database URLs remain
environment-only secrets, and classified errors do not expose connection or
server details.

The runtime role needs `SELECT` and `INSERT` on the new tables plus sequence
usage; it needs no update, delete, DDL, or provider credentials. Patches may
contain source-derived text, so access, backups, and future retention policy
must treat them as repository evidence rather than public telemetry.

## Validation

Unit tests cover contract round trip, forged identity rejection, service port
ordering, CLI ingestion/retrieval, public identity stability, and private
fingerprint stability. PostgreSQL integration tests cover empty and released-
schema migration, round trip, later-observation retry, conflicting reuse,
concurrent first ingestion, constraints, and process restart. `make validate`
and `make db-validate` provide the repository-wide and PostgreSQL acceptance
evidence.
