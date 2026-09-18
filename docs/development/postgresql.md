# PostgreSQL catalog development and operations

## TL;DR

- Argus uses PostgreSQL 17 and explicit, forward-only embedded migrations for
  the repository catalog.
- Set `ARGUS_DATABASE_URL` and run `go run ./cmd/migrate`; application startup
  never changes the schema.
- Use `go run ./cmd/catalog import ...` to idempotently persist a validated
  descriptor snapshot and `catalog get ...` to reconstruct it.
- Use `catalog list-tests` to query stable tests with capability filtering and
  deterministic keyset pagination.
- Use `catalog import-impact` and `catalog list-impact` to preserve evidence and
  evaluate supported, refuted, stale, or conflicting capability-to-test edges.
- The control plane stores authenticated GitHub delivery identity and bounded
  normalized change evidence with atomic exact-retry behavior.
- Database-changing work runs `make db-validate` against a disposable loopback
  PostgreSQL server configured by `ARGUS_TEST_POSTGRES_URL`.
- Migration files are immutable after merge. A checksum mismatch, unknown
  ledger entry, or ledger gap stops migration rather than guessing.

## Ownership and lifecycle

The control plane owns the `argus_catalog` schema and its data semantics.
Infrastructure owns the PostgreSQL service, database, roles, credentials, TLS,
backups, recovery, high availability, monitoring, and major-version lifecycle.
PostgreSQL 17 is the supported major for this slice; CI uses the exact 17.11
container image.

The Go adapter exposes runtime and migration capabilities separately.
`postgres.Store` implements the catalog snapshot, query, impact-evidence, and
change-ingestion ports and owns bounded read/write connections; opening it
never changes schema state. `postgres.Migrator` owns a single privileged
connection pool and can only apply the embedded migration chain. Commands
compose one capability or the other, so a runtime dependency cannot acquire
DDL authority through the same object.

[ADR-0004](../decisions/0004-use-postgresql-and-embedded-forward-migrations.md)
records the database, migration, transaction, identity, and Perfeng-reuse
decisions. The general migration policy remains
[SQL migration criteria](sql-migrations.md).
[ADR-0005](../decisions/0005-store-immutable-impact-evidence.md) defines the
append-only evidence, time, confidence, expiry, and conflict semantics.

## Apply migrations

Supply the migration role's URL through the environment rather than a command
argument, which could be exposed in process listings:

~~~sh
export ARGUS_DATABASE_URL='postgres://argus_migrator:...@db.example/argus'
go run ./cmd/migrate
~~~

In PowerShell:

~~~powershell
$env:ARGUS_DATABASE_URL = 'postgres://argus_migrator:...@db.example/argus'
go run ./cmd/migrate
~~~

The runner embeds timestamp-named SQL, takes a transaction-scoped advisory
lock, verifies the ordered filename/checksum ledger, and commits the entire
pending chain in one transaction. Repeating the command is a no-op. Migrations
never run as a side effect of opening the store or starting another command.

Do not edit a merged migration. Add a new UTC timestamp migration, test empty
and supported upgrade paths, and include forward recovery and application
rollback notes. A failed chain rolls back schema and ledger changes together.

## Import and read snapshots

After migrations, configure the runtime role and import the structurally and
semantically validated repository descriptor at a trusted immutable revision:

~~~sh
export ARGUS_DATABASE_URL='postgres://argus_runtime:...@db.example/argus'
go run ./cmd/catalog import \
  -revision 0123456789abcdef0123456789abcdef01234567 \
  contracts/fixtures/repository-descriptor/v1/valid/source-and-test-repositories.json
~~~

Read the snapshot by its immutable identity:

~~~sh
go run ./cmd/catalog get \
  -provider github \
  -host github.com \
  -repository-id R_orders_source_01 \
  -revision 0123456789abcdef0123456789abcdef01234567
~~~

The default revision algorithm is `git-sha1`; pass `-algorithm git-sha256` for
a full SHA-256 Git object ID. The descriptor API version defaults to
`argus.dev/repository-descriptor/v1`.

Writes canonicalize order-insensitive declaration lists before hashing and
persist the complete snapshot in one transaction. Retrying the same identity
and semantic content reports `created: false`; different content at the same
identity returns a conflict. Reads use a repeatable-read transaction and stable
key order. Callers may safely retry the exact same identity and content after
an ambiguous database failure.

## Query test catalog entries

Query one immutable snapshot without loading its complete component graph:

~~~sh
go run ./cmd/catalog list-tests \
  -provider github \
  -host github.com \
  -repository-id R_orders_source_01 \
  -revision 0123456789abcdef0123456789abcdef01234567 \
  -capability create-order \
  -page-size 50
~~~

The optional `-capability` value matches explicit test-to-capability mappings.
Omit it to include every cataloged test. Page size defaults to 50 and is bounded
to 1–200 entries. When the response contains a non-null `nextCursor`, pass the
value unchanged to `-cursor`; the page size may change on the continuation
request.

Pagination uses the stable natural identity `(test repository provider, host,
provider repository ID, suite key, test key)` and an exclusive keyset position.
It does not use mutable owner/name coordinates, database IDs, or offsets. SQL
comparison and ordering use the `C` collation so database locale cannot reorder
pages. The query runs in a read-only repeatable-read transaction, returning
snapshot metadata and entries from one consistent observation.

Cursors contain no database identifiers or credentials. They are versioned and
bound to the immutable snapshot and capability filter. Malformed, unsupported,
or query-incompatible cursors fail before the storage query. Cursors are
continuation tokens, not permanent test identifiers or authorization tokens.

A missing snapshot returns the catalog not-found outcome. An existing snapshot
with no matching capability mapping returns a successful empty page.

## Ingest and query impact evidence

An evidence bundle references an already persisted snapshot. The producer
repository revision and adapter form part of the immutable bundle identity:

~~~sh
go run ./cmd/catalog import-impact \
  contracts/fixtures/impact-evidence-bundle/v1/valid/orders-api.json
~~~

The write validates every capability and test reference before copying any
observation rows. The complete bundle commits in one transaction. Retrying the
same identity and canonical content reports `created: false`; changing content
at the same identity returns a conflict. Observation order is not semantic.

List evaluated relationships with an explicit UTC instant:

~~~sh
go run ./cmd/catalog list-impact \
  -provider github \
  -host github.com \
  -repository-id R_orders_source_01 \
  -revision 0123456789abcdef0123456789abcdef01234567 \
  -evaluated-at 2026-09-18T05:00:00Z \
  -capability create-order \
  -page-size 50
~~~

The optional capability filter and evaluation time are bound into the opaque
cursor. The query ignores observations from the future relative to that time,
marks evidence expired at `expiresAt`, and derives relationship state in the Go
application service. SQL performs a deterministic keyset read ordered by
capability key and stable test identity under `C` collation; it does not choose
between contradictory observations or aggregate confidence.

The evidence migration is additive and performs no data backfill, table scan,
or rewrite. Existing binaries remain compatible and application rollback
leaves unused tables in place. The runtime role needs the same table and
sequence privileges shown below; it does not need update or delete access on
evidence bundles or observations.

## Persist trusted change deliveries

The control plane claims `(delivery_provider, delivery_id)` in the same
transaction that stores the normalized change set and file rows. The signed
payload SHA-256 and canonical change-set SHA-256 distinguish an exact retry
from a conflicting reuse of immutable identity. Concurrent retries block on
the uniqueness constraint and converge on one stored result.

GitHub requests occur before this transaction begins. A slow or unavailable
provider therefore cannot hold database locks. Reads reconstruct the change set
under repeatable read, and a process restart preserves the retry result. The
migration is additive: it creates `change_sets` and `change_files`, performs no
backfill or table rewrite, and remains compatible with older binaries that do
not use these tables. Application rollback leaves the unused evidence intact.

## Least-privilege roles

The migration role owns the schema and ledger. The runtime role does not need
DDL, delete, truncate, or migration-ledger write access. After migrations, a
database owner can grant the runtime role:

~~~sql
GRANT USAGE ON SCHEMA argus_catalog TO argus_runtime;
GRANT SELECT, INSERT, UPDATE ON argus_catalog.repositories TO argus_runtime;
GRANT SELECT, INSERT ON ALL TABLES IN SCHEMA argus_catalog TO argus_runtime;
REVOKE INSERT, UPDATE, DELETE, TRUNCATE
    ON argus_catalog.schema_migrations FROM argus_runtime;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA argus_catalog TO argus_runtime;
~~~

Provision roles and passwords outside migrations. Production URLs require TLS
and certificate verification under the deployment policy. Errors deliberately
omit server messages and connection configuration; do not add database URLs to
logs, test output, shell history, or committed files.

## Local integration tests

Start any disposable PostgreSQL 17 server bound to loopback. With Docker, one
option is:

~~~sh
docker run --rm --name argus-postgres-test \
  -e POSTGRES_PASSWORD=argus_test \
  -p 127.0.0.1:5432:5432 \
  postgres:17.11
~~~

In another shell:

~~~sh
export ARGUS_TEST_POSTGRES_URL='postgres://postgres:argus_test@127.0.0.1:5432/postgres?sslmode=disable'
make db-validate
~~~

The integration suite refuses non-loopback hosts. It creates randomized
databases through the supplied administrative connection, exercises migration
and persistence behavior, closes all pools, and drops each database with
`FORCE`. `ARGUS_TEST_POSTGRES_URL` must use a `postgres://` or
`postgresql://` URL so the suite can retarget the connection to each generated
database; PostgreSQL keyword connection strings are rejected. Never point it
at a shared or production server.

The suite verifies empty and repeated migration, advisory-lock serialization,
checksum drift rejection, transactional rollback, relational constraints,
snapshot round trip, order-independent retry, immutable-content conflict,
concurrent ingestion, deterministic multi-page catalog queries, capability
filtering, empty/missing distinction, upgrade with representative data,
evidence retry/conflict and reference validation, temporal edge states,
concurrent evidence ingestion, delivery retry/conflict, concurrent delivery
claiming, and read-after-restart. Ordinary `make validate` remains
database-independent; CI runs `make db-validate` in a separate Ubuntu job with
an isolated PostgreSQL 17.11 service.

## Recovery and current limits

If migration reports drift, stop and compare the deployed binary, checked-in
migrations, and ledger. Do not rewrite the ledger or force a checksum. Restore
the expected binary/migration chain or deliver a reviewed forward repair.

A catalog import is append-only at the snapshot boundary. There is no delete
or in-place snapshot repair command in this slice. Backup/restore exercises,
evidence retention/deletion, source-precedence policy, HA, production SLOs,
latest-snapshot resolution, and a general authenticated network API remain
later work as recorded in the roadmap. The GitHub webhook is a narrow
provider-authenticated ingress; catalog evidence ingestion and queries remain
local administrative CLI boundaries until user/workload authorization exists.
