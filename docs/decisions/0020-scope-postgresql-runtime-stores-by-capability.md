# ADR-0020: Scope PostgreSQL runtime stores by capability

- Status: Proposed
- Date: 2026-10-01
- Milestone: M10 - Production readiness
- Deciders: Argus maintainers
- Supersedes: ADR-0004 and ADR-0006, only for PostgreSQL adapter package location and the omnibus runtime Store
- Superseded by: None

## TL;DR

- Keep one Argus PostgreSQL database, schema, runtime pool, and migration chain.
- Replace the all-port `Store` with catalog, change, execution, and adaptation
  store views over the same bounded runtime pool.
- Keep privileged migration opening separate from runtime opening.
- Compose only the views each command needs; do not grant a service another
  capability's persistence port merely because the database is shared.

## Context

The original catalog adapter acquired change, execution, and adaptation ports
as Argus grew. Its package name implied catalog ownership, while its runtime
`Store` implemented nine ports. Application services already depend on narrow
consumer-owned interfaces, but command composition passed the same broad
concrete object everywhere. This obscures write ownership and makes review of
new persistence behavior harder.

The schema and migration chain are shared product infrastructure. Splitting
databases, migration ledgers, or deployment units would add synchronization and
recovery risks without a corresponding product need.

## Decision

The `internal/postgres` adapter package owns Argus's one PostgreSQL schema and
driver-specific implementation. `Runtime` opens, verifies, bounds, and closes
one runtime pool; it implements no application persistence port. It creates
four concrete views: `CatalogStore`, `ChangeStore`, `ExecutionStore`, and
`AdaptationStore`. Each view implements only its capability's consumer ports.
Command composition roots may combine views when a transitional command spans
capabilities, but application services receive only their required view.

`Migrator` remains separately opened with its own single-connection pool and
the unchanged embedded SQL bytes, ordering, checksum ledger, advisory lock,
and whole-chain transaction. Runtime opening never performs DDL. Error
classification remains private to the adapter and translates into the owning
capability's stable errors.

This decision supersedes only the old package location and omnibus store
shape in ADR-0004 and ADR-0006. Their database, schema, migration, transaction,
identity, and single-deployable decisions remain in force.

## Alternatives considered

### Separate database or service for each capability

- Benefits: Strong independent deployment and credential boundaries.
- Costs and risks: Distributed transactions, duplicated migrations, and new
  release/recovery coordination for tightly coupled evidence.
- Reason not selected: No independent operational lifecycle justifies it.

### Keep the catalog-named all-port store

- Benefits: Minimal code movement.
- Costs and risks: False ownership signal and ambient access to unrelated
  persistence ports at every composition site.
- Reason not selected: The growing write surface is the problem being fixed.

### Split every SQL file into a separately compiled Go adapter package now

- Benefits: Package-level compiler enforcement of SQL ownership.
- Costs and risks: Shared repository identity, transaction, migration, and
  SQL error primitives would need a broad exported platform API or duplication.
- Reason not selected: Scoped concrete method sets and command wiring provide
  a useful first boundary without exposing low-level database internals.

## Consequences

### Positive

- Each application service receives a persistence adapter with only its own
  ports, while related commands can deliberately share one bounded pool.
- The package no longer claims catalog ownership of all Argus data.
- Runtime and migration authority remain visibly separate.

### Negative

- The PostgreSQL implementation is still one Go package, so SQL ownership is
  enforced by concrete method sets, tests, and review rather than separate
  import boundaries. Revisit package splitting if independent ownership or
  repeated cross-capability changes justify a small shared platform API.

### Neutral or follow-up

- Authenticated network APIs may eventually replace transitional direct-DB
  commands; this decision does not expand their authorization model.

## Compatibility and migration

No SQL migration, table, row encoding, checksum, public contract, JSON output,
cursor, environment variable, or command syntax changes. Existing runtime
credentials and rollout procedures remain valid. This is an internal Go import
and construction change; normal application rollback uses the same schema.

## Security and operations

Only `cmd/migrate` opens the privileged migrator. Runtime pools remain bounded
and closed by their composition roots. Each command opens a single pool even
when it needs two capability views. No credential is copied into a store view
or included in an error.

## Validation

Compile-time port assertions and negative method-set tests check view scope.
Unit and race tests check canonical identity and error handling. The integration
suite retains migration-chain, cross-capability foreign-key, retry, conflict,
restart, and constraint coverage against disposable PostgreSQL. `make validate`
and `make db-validate` are the acceptance checks.
