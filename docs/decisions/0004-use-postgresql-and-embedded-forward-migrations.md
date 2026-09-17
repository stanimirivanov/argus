# ADR-0004: Use PostgreSQL and embedded forward migrations

- Status: Accepted
- Date: 2026-09-17
- Milestone: M03 - Repository catalog
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Use PostgreSQL 17 as the first catalog persistence engine and pgx v5 as the
  Go driver.
- Keep SQL migrations beside the catalog adapter and embed them in the migration
  command.
- Apply forward-only migrations explicitly in one transaction under a
  transaction-scoped advisory lock.
- Record and verify migration filename and SHA-256 checksum history; reject
  unknown versions, changed bytes, and ledger gaps.
- Store normalized catalog identities and mappings relationally, and make
  descriptor ingestion an immutable, idempotent snapshot write.
- Reuse Perfeng's proven migration pattern, but do not create a shared library
  until repeated maintenance and lifecycle ownership justify extraction.

## Context

M02 can validate a repository descriptor and convert it into normalized Go
catalog state, but that state disappears when the command exits. M03 needs a
durable, queryable source of catalog identity for later impact analysis and
test selection. The first persistence slice must represent one source
repository and a separate test repository without weakening repository, suite,
test, revision, or mapping scope.

The migration policy requires a selected runner before the first production
migration. Argus also needs empty-database, repeat, checksum, rollback,
concurrency, constraint, and restart evidence. Automatic framework schema
creation would make those behaviors implicit and would couple database changes
to ordinary process startup.

Perfeng's Go control plane already demonstrates a compact pattern: embedded SQL,
a checksum ledger, a transaction-scoped PostgreSQL advisory lock, one
transaction for the chain, and an explicit administrative command. The pattern
is relevant to Argus, but the products still have independent schemas,
deployments, release lifecycles, and owners under ADR-0001.

## Decision

### Database and driver

Argus uses PostgreSQL major version 17 for the initial catalog. Production and
CI MUST run a supported current minor in that major; CI initially verifies
17.11. A later major upgrade requires compatibility testing and an operational
plan, not a schema rewrite disguised as an application change.

Go accesses PostgreSQL through the stable pgx v5 module and its native bounded
pool. Database records remain private to the persistence adapter. Domain
packages do not import pgx or SQL types.

The control plane owns the `argus_catalog` schema. Infrastructure owns the
database service, backups, credentials, role provisioning, TLS, high
availability, and major-version lifecycle. Migration and runtime identities
SHOULD be separate outside disposable development and CI databases.

### Migration runner

The migration runner is implemented in the catalog PostgreSQL adapter and
versioned with the Argus binary. It embeds SQL files named with a 14-digit UTC
timestamp and a descriptive lower-snake-case suffix. Lexical order is migration
order.

Migration is an explicit administrative operation. Opening a runtime store
MUST NOT apply migrations. The runner:

1. starts one transaction and enables synchronous commit;
2. takes one Argus-specific transaction-scoped advisory lock;
3. bootstraps the owned schema and migration ledger;
4. validates known filenames, ordering, ledger continuity, and SHA-256 values;
5. applies pending SQL and records its checksum in the same transaction; and
6. commits the entire chain or rolls it all back.

`CREATE ... IF NOT EXISTS` is permitted only for the runner-owned schema and
ledger bootstrap needed to inspect history on repeated execution. Production
migrations MUST NOT use conditional DDL to conceal drift. The runner validates
the ledger structure through its subsequent queries and treats disagreement as
failure.

There are no automatic down migrations. Application rollback keeps the additive
schema and deploys compatible code. A destructive correction is a new forward
migration with an explicit recovery plan.

### Catalog persistence

Repository identity is relational and unique by provider, host, and
provider-assigned repository identifier. Mutable owner and name values are
stored as the latest observed coordinates, while each immutable catalog
snapshot retains the coordinates present in that descriptor.

A catalog snapshot is uniquely identified by source repository identity,
revision algorithm, full revision digest, and descriptor API version. It stores
a SHA-256 fingerprint of a canonical normalized snapshot. Repeating the same
identity and content is a no-op; repeating the identity with different content
is a conflict and MUST NOT overwrite the first snapshot.

Capabilities, components, test suites, stable tests, and their mappings are
stored in normalized tables with primary, unique, check, and foreign-key
constraints. Snapshot-owned rows cascade only with their snapshot because they
have no independent lifecycle. Repository rows are restricted from deletion
while referenced. Snapshot ingestion is one transaction and contains no
external calls.

Reads reconstruct domain snapshots in stable key order inside a read-only,
repeatable-read transaction. Persistence ordering is deterministic and does not
claim that declaration order carries product meaning.

### Verification environment

Database integration tests create random disposable databases only from an
explicit loopback administrative DSN, migrate them, and drop only the generated
test database. They reject remote hosts and unsafe database names.

The dedicated GitHub Actions job uses the official PostgreSQL 17.11 service
image on Ubuntu. Ordinary Go, TypeScript, and cross-platform validation remains
on Ubuntu and Windows. Developers without disposable PostgreSQL run the normal
suite and report the database-specific command as not run; schema-changing
pull requests still require the dedicated database job before merge.

## Alternatives considered

### Use a third-party migration framework

- Benefits: Familiar CLI, broader feature set, and less runner code in Argus.
- Costs and risks: Another executable lifecycle, directive syntax, dependency
  graph, and compatibility surface for requirements that are currently small.
- Reason not selected: Embedded ordered SQL plus pgx is sufficient for the
  required transactional, checksum, and locking behavior.

### Reuse the Perfeng runner as source or a shared library immediately

- Benefits: Less duplicated migration machinery and aligned behavior.
- Costs and risks: A source-layout or module dependency would couple independent
  product releases. A shared package still needs an owner, versioning, support,
  and coordinated rollout.
- Reason not selected: Argus reuses the validated pattern. Extraction remains
  available when repeated changes prove stable common semantics and ownership.

### Store the complete descriptor only as JSONB

- Benefits: Minimal schema and straightforward round-trip storage.
- Costs and risks: Repository, suite, test, and mapping integrity would be left
  to application code; later impact queries would repeatedly parse documents.
- Reason not selected: The catalog's stable identities and relationships are
  relational data. The independently versioned input document remains the
  contract, not the database model.

### Use SQLite for local simplicity

- Benefits: No service dependency and easy cross-platform tests.
- Costs and risks: Different concurrency, constraint, locking, SQL, and
  operational behavior from the intended production database.
- Reason not selected: It would verify a substitute rather than the production
  persistence semantics.

### Run migrations automatically on control-plane startup

- Benefits: Fewer deployment steps.
- Costs and risks: Runtime credentials would need DDL authority, multiple
  replicas could race startup, and schema failure would be coupled to service
  availability.
- Reason not selected: Explicit migration jobs preserve least privilege and a
  reviewable deployment boundary.

## Consequences

### Positive

- Catalog state survives process restart and is queryable by stable identity.
- Database constraints enforce scope and referential integrity under
  concurrency.
- Migration bytes and history are tamper-evident.
- Repeated descriptor delivery is safe, while conflicting content remains
  visible as an error.
- The Argus and Perfeng implementations can now be compared using real usage
  before proposing a shared migration package.

### Negative

- Contributors need disposable PostgreSQL for database-specific verification.
- The repository owns a small migration runner that must be maintained and
  security-reviewed.
- Relational normalization requires explicit row/domain mapping and several
  tables for one descriptor.
- The first slice does not yet provide pagination, history retention controls,
  or an external catalog API.

### Neutral or follow-up

- Impact edges, provenance, confidence, expiry, conflict reporting, and stale
  mapping policy remain later M03 slices.
- Runtime roles and grants are provisioned by deployment infrastructure; the
  migration does not create credentials.
- Backup, restore, retention, HA, and production SLO validation remain M10 work.
- A shared Argus–Perfeng migration library still requires ADR-0001's ownership,
  compatibility, and repeated-maintenance evidence.

## Compatibility and migration

This is the first Argus database schema, so there is no supported predecessor
to upgrade. Verification covers an empty database, repeat execution, concurrent
execution, and transactional rollback. Future schema changes MUST test upgrade
from every still-supported released migration ledger with representative data.

The initial schema is additive relative to the stateless control plane. Rolling
back the application leaves unused tables in place. Once the migration reaches
a shared persistent environment, its filename and bytes are immutable.

## Security and operations

Database URLs are secrets and are accepted through environment configuration;
commands and errors MUST NOT print them. Production connections require TLS and
certificate verification according to deployment policy. The migration login
needs schema ownership; the runtime login needs only catalog `SELECT` and
`INSERT`, plus repository-coordinate `UPDATE` for newly observed snapshots.

Operations use bounded pools and timeouts. Synchronous commit is enabled for
catalog writes, while server fsync, durable storage, replication, backup, and
recovery remain deployment responsibilities. A commit error is an uncertain
outcome; callers retry only the same immutable snapshot identity and content.

The schema stores repository metadata and test names, not credentials, source
contents, customer fixtures, or test artifacts. Provider identifiers and paths
remain data and do not grant repository or filesystem authority.

## Validation

The decision is verified by:

- migration filename and checksum validation;
- empty-database, repeat, concurrent, drift, and rollback migration tests;
- database constraint and transaction tests;
- exact-retry, conflict, deterministic-read, and process-restart tests;
- a source repository with a separate functional-test repository;
- normal `make validate` on Ubuntu 24.04 and Windows Server 2025; and
- `make db-validate` against PostgreSQL 17.11 in a required Ubuntu CI job.

Primary references:

- [PostgreSQL 17 advisory locks](https://www.postgresql.org/docs/17/explicit-locking.html)
- [pgx v5](https://pkg.go.dev/github.com/jackc/pgx/v5)
- [GitHub PostgreSQL service containers](https://docs.github.com/en/actions/tutorials/use-containerized-services/create-postgresql-service-containers)
