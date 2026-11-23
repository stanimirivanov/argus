# ADR-0005: Store immutable impact evidence and derive edge state at query time

- Status: Accepted
- Date: 2026-09-18
- Milestone: M03 - Repository catalog
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Store producer evidence as immutable, idempotent bundles scoped to one
  immutable catalog snapshot.
- Preserve support and refutation observations independently; never overwrite
  contradictory evidence or silently select a winner.
- Keep confidence as producer-specific basis-point metadata rather than a
  universal policy threshold.
- Derive supported, refuted, stale, and conflicting states at an explicit query
  instant so historical decisions remain reproducible.
- Treat producer observation time, optional expiry, and database ingestion time
  as distinct instants.

## Context

The catalog can persist repository descriptors and enumerate stable tests, but
the direct test-to-capability arrays are not yet an evidence model. M04 and M05
need to combine explicit declarations with later static, dynamic, historical,
and reviewer-confirmed observations. Those sources can expire or contradict
one another. Replacing an older mapping with the newest row would destroy the
evidence needed to explain a selection, identify stale knowledge, or review a
conflict.

Confidence also has no universal meaning across producers. A static analyzer's
score, a coverage observation, and an owner declaration are not calibrated on
the same population. Collapsing them into one catalog score would create a
policy decision inside persistence before selection policy exists.

## Decision

### Immutable bundle identity

Argus ingests `argus.dev/impact-evidence-bundle/v1` documents. A bundle is
scoped to one immutable catalog snapshot and identified by that snapshot,
producer repository identity, producer revision, producer adapter, and evidence
contract major. Repeating the identity and canonical content is an exact retry;
different content for the same identity is a conflict and cannot overwrite the
first observation.

The producer records an event-time `observedAt` and MAY declare an `expiresAt`.
No expiry means the producer has not declared one; it does not prove perpetual
correctness. PostgreSQL separately records ingestion time. Observations within a
bundle use stable local keys and refer to capabilities and tests that must exist
in the target snapshot.

### Evidence and confidence

Each observation asserts either `supports` or `refutes` for one
capability-to-test relation and identifies one evidence method: explicit,
static, dynamic, historical, or reviewer-confirmed. Both positive and negative
evidence are first-class. Missing evidence is represented by no observation and
is never converted into a refutation.

Confidence is an integer from 0 through 10,000 basis points plus a required
rationale. It describes the producer's own uncertainty. Catalog persistence
does not compare scores across producers, apply thresholds, or grant authority.
Later versioned policy may use evidence type, source, confidence, and repository
configuration explicitly.

### Query-time state

Impact queries require an explicit UTC evaluation instant and ignore evidence
observed after that instant. Evidence whose expiry is at or before the instant
is expired. Argus derives relation state without mutating stored observations:

- `supported`: active support and no active refutation;
- `refuted`: active refutation and no active support;
- `conflicting`: active support and active refutation; and
- `stale`: visible observations exist but all are expired.

Every result returns the individual evidence records. A conflict includes the
active supporting and refuting counts but does not choose a winner. This M03
slice deliberately defines no source-precedence policy.

## Alternatives considered

### Keep only descriptor arrays

- Benefits: No additional contract, tables, or ingestion command.
- Costs and risks: Cannot distinguish provenance, observation time, expiry,
  negative evidence, or contradictions; later analysis would invent history.
- Reason not selected: Explainability and stale/conflict reporting require the
  observations that produced a relation.

### Store one mutable current edge

- Benefits: Simple reads and small storage footprint.
- Costs and risks: Last-write-wins destroys contradictions and prevents
  historical reproduction, delayed review, and audit.
- Reason not selected: It conflicts with Argus's evidence and conservative
  fallback principles.

### Aggregate confidence during ingestion

- Benefits: Consumers receive one convenient score.
- Costs and risks: Scores from different methods are not calibrated together;
  aggregation embeds undeclared selection policy in storage.
- Reason not selected: Evidence remains portable while policy evolves and can
  be evaluated separately.

### Derive state from database current time

- Benefits: Shorter query input and SQL.
- Costs and risks: Repeating the same query later can change its meaning and a
  cursor can cross an expiry boundary.
- Reason not selected: An explicit evaluation instant makes pages and decisions
  reproducible.

## Consequences

### Positive

- Contradictions and expired knowledge become observable instead of destructive
  updates.
- Every relationship remains traceable to an immutable producer revision,
  adapter, rationale, and time interval.
- M04/M05 can consume the same evidence without depending on PostgreSQL records
  or one premature confidence formula.
- Query cursors can bind to evaluation time and remain stable across pages.

### Negative

- Storage grows with observations rather than only current state.
- Producers must choose stable bundle and observation identities and meaningful
  expiry semantics.
- Queries require grouping evidence by edge before application policy derives
  state.

### Neutral or follow-up

- Source precedence, repository policy, calibration, retention, and deletion
  remain separate versioned decisions.
- M04 adds change-derived observations; M05 uses edge state in selection with
  conservative fallback.
- Authentication and repository authorization remain required before evidence
  ingestion is exposed through a network API.

## Compatibility and migration

The change adds new tables and versioned contracts. Existing binaries continue
to use the original catalog schema and do not write evidence. Application
rollback leaves unused additive tables in place. The migration has no backfill,
table rewrite, or row scan; it takes ordinary short-lived catalog DDL locks.

Breaking contract semantics require a parallel major. Database constraint
changes follow expand/migrate/verify/contract and preserve the immutable bundle
ledger.

## Security and operations

Evidence documents are untrusted input. Argus validates schema, semantic time
ordering, closed sets, bounds, snapshot references, and foreign keys before the
bundle commits. Adapter names and rationales are data, never executable
commands. Database URLs remain secret and errors do not expose them.

The runtime database role needs `INSERT` and `SELECT` on the new tables and
identity sequence plus the existing repository-coordinate update privilege.
Bundles and observations are append-only. Retention and deletion require a
future explicit policy because evidence may support historical decisions.

## Validation

The decision is verified by:

- Effect Schema and generated JSON Schema compatibility fixtures;
- Go semantic tests for timestamp ordering, uniqueness, bounds, and closed
  values;
- PostgreSQL migration-chain and constraint tests;
- exact retry and immutable-content conflict tests;
- supported, refuted, stale, and conflicting query tests at fixed instants;
- deterministic multi-page reads and query-bound cursor tests;
- process-restart integration coverage;
- `make validate`; and
- `make db-validate` against PostgreSQL 17.11.
