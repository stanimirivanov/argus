# ADR-0011: Store immutable attempts and compare explicit shadow pairs

- Status: Accepted
- Date: 2026-09-26
- Milestone: M05 - Functional API selection
- Deciders: Argus maintainers
- Supersedes:
- Superseded by:

## TL;DR

- Persist each normalized execution attempt as immutable relational evidence.
- Treat the caller-provided attempt ID as a global idempotency key: exact
  retries succeed without another row and divergent reuse is a conflict.
- Derive a shadow report only from two explicitly named compatible attempts;
  never select an implicit “latest” control.
- Store artifact metadata and checksums, while external artifact storage owns
  byte upload, availability, and retention.
- Keep full-suite execution authoritative while Argus measures selected-stage
  duration reduction and failure recall.

## Context

The first M05 execution slice emits normalized attempt documents from a
CI-local functional API adapter. Those documents must survive CI process
lifetimes before Argus can compare an early selected run with its later
full-suite control. Retries, concurrent ingestion, repository renames, and
delayed controls must not overwrite or accidentally pair evidence.

The comparison is evaluation evidence, not a release decision. A full-suite
failure omitted by selection and a test that ran early but did not reproduce
its later failure have different diagnostic meanings. Both must remain visible.

## Decision

Argus stores normalized execution attempts, per-test results, and artifact
references in append-only relational tables. The public attempt ID is globally
unique and is the ingestion idempotency key. The adapter computes a SHA-256
over canonical validated domain content. An exact retry returns the existing
identity; different content under the same attempt ID returns a conflict. A
new attempt ID always represents distinct evidence, even when every other field
matches an earlier retry.

Artifact rows contain a key, kind, absolute credential-free URI, and checksum.
Their presence does not prove that external bytes are currently retrievable.
Upload and retention remain the responsibility of the CI and artifact system.

The shadow application service accepts one selected attempt ID and one
full-suite attempt ID. It requires the stages to be `selected` and
`full-suite`, and requires equal manifest digest, test-repository identity,
test revision, and adapter ID/version. The full-suite test set must be a
superset of the selected set. Argus does not find either attempt by recency.

`failed` and `error` are failure signals for recall. A full-suite failure is
caught when the same stable test identity also failed or errored in the
selected attempt. A missing selected result is reported as `not-selected`; a
selected pass or skip followed by a full-suite failure is `not-reproduced`.
Failure recall is reported in integer basis points and is null when the full
suite has no failures. Duration reduction is full-suite summed test duration
minus selected summed test duration and may be negative.

Reports are derived on demand and are not persisted in this slice. The
versioned report contract makes the comparison portable without introducing a
second mutable source of truth.

## Alternatives considered

### Store each attempt as JSONB

- Benefits: fewer tables and direct retention of the wire document.
- Costs and risks: weaker queryable invariants, more difficult test-identity
  joins, and format concerns leak into persistence.
- Reason not selected: attempts, results, and artifacts have stable relational
  meaning and database-enforceable constraints.

### Upsert the latest attempt for a manifest

- Benefits: smaller data volume and simple “current result” queries.
- Costs and risks: destroys retry history, makes delayed evidence ambiguous,
  and permits controls to silently change beneath a report.
- Reason not selected: evaluation and audit require immutable provenance.

### Pair the selected attempt with the latest matching full-suite run

- Benefits: less input required from callers.
- Costs and risks: race-dependent reports and accidental pairing across CI
  reruns or adapter upgrades.
- Reason not selected: explicit IDs are reproducible and reviewable.

## Consequences

### Positive

- Concurrent exact retries converge through a database uniqueness constraint.
- Stored evidence can be reconstructed and validated independently of CI.
- Misses are first-class evidence rather than aggregate metrics only.
- Comparison remains deterministic across restarts and delayed controls.

### Negative

- Relational child rows increase write volume and require a retention policy
  before production-scale history accumulates.
- Callers must retain and pass both attempt IDs.
- External artifact availability cannot be inferred from metadata ingestion.

### Neutral or follow-up

- Automated heterogeneous group planning remains separate from evidence
  ingestion.
- Retention, report aggregation, and predictive promotion policy require
  measured operating data and later decisions.

## Compatibility and migration

The migration only creates empty tables and constraints. It does
not scan, rewrite, or backfill existing catalog data. Existing binaries ignore
the new tables, so application rollback leaves unused immutable evidence in
place. Schema rollback is forward recovery, not destructive down migration.

On an empty current database the DDL takes only catalog locks while creating
new objects. There is no existing table-size-dependent lock duration, data WAL,
or replication-heavy backfill. The runtime role needs insert and select access
to the new tables and sequence under the existing least-privilege grants.

## Security and operations

Attempt and failure metadata are untrusted at ingestion and pass structural
and domain validation before storage. Failure messages are bounded but may
still contain sensitive application content; adapters must redact secrets and
customer data. Artifact URIs cannot embed user credentials. Raw database
errors and connection configuration do not cross the PostgreSQL adapter.

The write transaction contains no network or artifact-system calls. It claims
the parent identity and writes all child rows atomically with bounded lock and
operation timeouts. PostgreSQL 17 remains the supported database major.

## Validation

- Generate and test the Effect Schema/JSON Schema report contract and corpus.
- Verify exact retry, divergent conflict, concurrent first write, atomic child
  persistence, constraint rejection, repeatable-read reconstruction, and
  restart behavior against PostgreSQL 17.
- Compare a compatible selected/full-suite pair and reject incompatible or
  non-superset pairs in application tests.
- Run the full repository validation and migration-chain suites.
