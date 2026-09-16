# SQL migration criteria

## TL;DR

- PostgreSQL migrations are immutable, reviewed production code and the source
  of truth for the persisted schema.
- Keep each migration focused, forward-compatible, and safe for mixed-version
  deployments.
- Encode durable invariants in the database and document domain meaning.
- Use expand, migrate, verify, and contract for breaking changes.
- Analyze locks, rewrites, WAL, backfills, recovery, and rollback before merge.
- Verify the complete chain from an empty database and from every supported
  released schema.

## Policy strength

Normative terms use the meanings defined in
[CONTRIBUTING.md](../../CONTRIBUTING.md#policy-language-and-sources-of-truth).
MUST and MUST NOT are reviewable requirements; SHOULD and SHOULD NOT are strong
defaults that require a recorded reason to deviate; MAY is optional. Other
wording provides design and review guidance. The checks under
[Required verification](#required-verification) are mandatory as specified in
that section.

## Ownership and tooling

PostgreSQL migration files own production schema evolution. Application
framework auto-create or auto-update features are disabled outside disposable
tests.

Select and record the migration runner in an ADR before the first production
migration. Pin its version and run the same validation in development and CI.
Use versioned SQL migrations with UTC timestamp identifiers and descriptive
lower_snake_case names in the exact syntax required by that runner. Timestamp
versions avoid collisions across concurrent branches.

Immediately before creating a migration, refresh the branch as the current
workflow allows and inspect the migration directory and runner history. Generate
the UTC timestamp from that fresh state; do not copy a timestamp from an example
or stale plan. If concurrent work still produces a duplicate version, rebase or
otherwise refresh, assign the later unshared migration a new timestamp, and
update references and checksums. Do not combine unrelated migrations or force a
numbering conflict through merge resolution. Once a migration reaches a shared
persistent environment, the immutability rule below takes precedence.

Never edit, rename, reorder, or delete a versioned migration after it reaches a
persistent shared environment. Correct it with a new forward migration.
Checksums and migration history are compatibility evidence, not obstacles to
work around.

## Scope and evolution

- One migration represents one coherent schema capability or data transition.
  Split unrelated DDL and formatting.
- Migrations and the application behavior that depends on them share one
  delivery plan, even when an expand/contract sequence requires several
  releases.
- Prefer additive changes compatible with both the current and next application
  version.
- Use expand, migrate, verify, and contract for column renames, type changes,
  table replacement, constraint tightening, or removal.
- Do not stop writing an old representation in the same deployment that first
  introduces its replacement unless atomic deployment is proven and documented.
- Keep migrations transactional. Isolate operations that cannot run in a
  transaction and document retry and recovery behavior.
- Do not use IF EXISTS or IF NOT EXISTS to hide unexpected drift. Tolerance is
  permitted only for a documented compatibility condition.
- Prefer SQL. A code-driven backfill is justified only when SQL cannot express
  it safely; pin its implementation revision and make progress, retries, and
  replay behavior explicit.

## SQL style and schema design

- Use unquoted lower_snake_case identifiers and uppercase SQL keywords and
  built-in types.
- Qualify product objects with their schema where search_path ambiguity is
  possible.
- Use plural table names and singular column names unless established domain
  vocabulary requires otherwise.
- Give constraints, indexes, and triggers stable descriptive names using pk_,
  fk_, uq_, ck_, ix_, and trg_ prefixes.
- Every table has an explicit primary key.
- Use application-generated UUIDs for externally referenced domain identity.
  Use generated identity columns for internal sequences; do not introduce
  serial.
- Prefer TEXT unless length is a real domain invariant. Enforce real limits
  with a named CHECK constraint and domain validation.
- Store instants as TIMESTAMPTZ in UTC. Distinguish domain event time from
  ingestion and persistence time.
- Use NUMERIC for exact quantities. Store currency or unit explicitly.
- Columns are NOT NULL by default. Every nullable column has one documented
  meaning; NULL must not ambiguously mean unknown, absent, inapplicable,
  redacted, and deleted.
- State deletion behavior explicitly. Cascade only when the child has no
  independent lifecycle or retention rule.
- Keep frequently queried, joined, constrained, or sorted fields relational.
  JSONB is for genuinely open or independently versioned payload content, not a
  substitute for schema design.
- Event, audit, and decision evidence is append-only unless an accepted ADR
  defines correction and retention semantics.

## Integrity, tenancy, and access

- Enforce stable invariants with primary keys, foreign keys, unique,
  exclusion, and check constraints. Application validation improves messages
  but does not replace integrity under concurrency.
- Include tenant or security scope in owned primary, unique, and foreign-key
  paths so the database cannot create a cross-scope association.
- Review privileges and row-level security whenever a migration creates a new
  data boundary. Application predicates do not replace database authorization
  where direct access is possible.
- Do not store credentials. Minimize personal and sensitive data and define
  retention, deletion, export, and audit behavior.
- When a database constraint mirrors a Go, Python, or TypeScript closed set,
  update both through a compatible sequence and add a persistence/contract
  test.

## Indexes and queries

- Add indexes for demonstrated filter, join, ordering, uniqueness, or
  foreign-key access paths. PostgreSQL does not automatically index the
  referencing side of a foreign key.
- Explain composite column order, included columns, expression indexes, and
  partial predicates using their target query.
- Avoid speculative indexes; every index adds write, storage, vacuum, and
  migration cost.
- Batch homogeneous work with set-based SQL rather than executing a statement
  in an application loop.
- Keep database row representations private to persistence adapters and
  reconstruct domain values through validated constructors.
- Document non-obvious query shape, locking, isolation, temporal selection,
  batching, ordering, and deliberate multi-query decisions.

## Documentation inside migrations

- Start a non-trivial migration with a compact summary of its capability,
  authoritative and derived data, ownership, mutability, compatibility, and
  required write order.
- Divide substantial files into named conceptual sections.
- Comment decisions that are not evident from the DDL, including NULL meaning,
  intentionally redundant constraints, lock choices, and version pinning.
- Add COMMENT ON TABLE and selective COMMENT ON COLUMN/FUNCTION statements for
  durable meaning useful to operators and analysts.
- Do not translate every identifier into prose. Misleading comments are schema
  defects and change with the DDL.

## Operational safety

Before review, estimate and document as applicable:

- lock level and expected duration;
- table and index size;
- table rewrite and scan behavior;
- WAL generation and replication lag;
- backfill volume, rate, checkpoints, resumption, and observability;
- disk headroom and vacuum implications;
- mixed-version application behavior;
- backup and restore prerequisites;
- application rollback with the new schema still present; and
- forward recovery if the migration partially succeeds.

Large backfills are resumable, idempotent, observable, and separated from
long-held schema locks. Risky index builds use the appropriate online approach
in an isolated migration with explicit failure cleanup.

Down migrations are not the production rollback strategy for destructive
changes. Production rollback normally deploys compatible application code and
rolls the schema forward. A runner-required down section must either be safe and
tested or explicitly unsupported according to repository policy.

## Data migrations and seeds

- Seed only deterministic reference data required for application correctness.
  Demo accounts, examples, customer data, and test fixtures do not belong in
  production migrations.
- A data transformation states source of truth, conflict handling, ordering,
  batching, restart identity, and completion verification.
- Idempotent append-only writes compare every immutable field before treating a
  key conflict as an exact retry.
- Do not use a no-op update merely to force visibility or conceal an identity
  collision.
- Validate counts, checksums, invariants, or domain-specific reconciliation
  before the contract/removal phase.

## Required verification

Every schema change MUST satisfy all applicable checks below. A check that
cannot run follows the constrained-environment protocol in
[CONTRIBUTING.md](../../CONTRIBUTING.md#verification-and-constrained-environments)
and MUST NOT be reported as passed.

- applies the complete migration chain to an empty database on the supported
  PostgreSQL version;
- upgrades from the latest supported released schema with representative data;
- validates migration checksums and ordering;
- runs application compilation/type checking and affected query tests;
- includes integration tests for new constraints, transactions, concurrency,
  scope isolation, and row-to-domain mapping;
- proves old/new application compatibility for expand/contract work;
- records any rollout or recovery step that cannot be automated; and
- confirms that no credentials, customer data, or production evidence is
  present.

Run repository-specific formatting, static analysis, tests, race/concurrency
checks, and vulnerability checks in addition to database verification.

## Review questions

- Is the migration immutable, focused, and understandable without application
  archaeology?
- Does the database enforce the invariants it can enforce reliably?
- Can current and next application versions coexist with this schema?
- Are locks, scans, rewrites, indexes, WAL, and backfills operationally safe?
- Can a failed deployment recover without rewriting migration history?
- Are domain meaning, ownership, time, units, NULL behavior, and retention
  clear?
- Are empty-database and supported-upgrade paths both tested?

## Primary references

- [PostgreSQL constraints](https://www.postgresql.org/docs/current/ddl-constraints.html)
- [PostgreSQL explicit locking](https://www.postgresql.org/docs/current/explicit-locking.html)
- [PostgreSQL index creation](https://www.postgresql.org/docs/current/sql-createindex.html)
- [PostgreSQL date/time types](https://www.postgresql.org/docs/current/datatype-datetime.html)
