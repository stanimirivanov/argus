# ADR-0006: Enforce capability-oriented hexagonal boundaries

- Status: Accepted
- Date: 2026-09-18
- Milestone: M03 - Repository catalog
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Keep shared catalog vocabulary and invariants in `internal/catalog`.
- Give each application capability its own package and let that package own the
  ports it consumes.
- Place driving and driven adapters under `internal/catalog/adapters`; keep
  `cmd` packages as composition roots only.
- Enforce dependency direction with a repository test rather than relying only
  on review conventions.
- Do not create generic `domain`, `ports`, or `services` layers, a dependency
  injection framework, or a service boundary for every package.

## Context

ADR-0001 requires a capability-oriented core, consumer-owned ports, and inward
dependencies. The first repository-catalog slices followed those principles in
behavior, but most application services, ports, cursor policy, and shared
domain values accumulated in one `internal/catalog` package. Contract adapters
and PostgreSQL were peers of that package, while `cmd/catalog` combined
argument parsing, transport conversion, application orchestration, and
infrastructure construction.

That layout remained testable, but package imports did not make the intended
boundaries obvious. A contributor could add SQL to an application service,
make the CLI select storage policy, or let shared domain values depend on a
capability package without receiving deterministic feedback. The upcoming
change-impact and selection work would multiply that ambiguity.

## Decision

Argus uses a capability-oriented hexagonal modular monolith.

`internal/catalog` owns only catalog concepts shared by more than one
capability: repository and revision identity, snapshot and test identity,
canonical catalog representation, invariants, and stable catalog outcome
errors. It MUST NOT import application capabilities, contracts, database
drivers, or adapters.

Each application capability owns its use-case service and the narrow driven
port required by that service:

- `internal/catalog/snapshot` owns snapshot ingestion and retrieval plus its
  `Store` port;
- `internal/catalog/testquery` owns bounded test queries, cursor semantics, and
  its reader port; and
- `internal/catalog/impact` owns evidence ingestion, impact-edge queries,
  temporal/conflict policy, cursors, and its store/reader ports.

Application capability packages MAY import the shared catalog domain. They
MUST NOT import contracts, PostgreSQL, CLI code, or another infrastructure
adapter. A capability MUST NOT import another capability merely to reuse an
incidental helper; genuinely shared domain language moves inward only when at
least two capabilities need the same semantics.

Adapters live below `internal/catalog/adapters`. Contract adapters convert
versioned generated DTOs into application/domain values. The PostgreSQL
adapter implements application-owned ports and owns SQL, driver error mapping,
transactions, fingerprints, and migrations. The catalog CLI adapter owns
argument parsing, local file input, and versioned output conversion. It
receives a composite runtime factory and MUST NOT import PostgreSQL directly.

Executable packages under `cmd` select concrete adapters, supply process
configuration and cancellation, and report terminal errors. They contain no
use-case policy. The migration command remains a separate composition root for
the privileged `Migrator`; opening the runtime `Store` still cannot migrate a
schema.

An architecture test parses production imports and fails when the shared
domain points outward, an application capability imports infrastructure, or
the CLI adapter selects PostgreSQL. New capability boundaries MUST extend this
test when review alone would permit an invalid dependency.

This decision governs package dependency direction, not deployment topology.
It does not require one service per capability, a global `ports` package,
generic `domain/application/infrastructure` directories, reflection-based
dependency injection, or interfaces that have no substitutable boundary.

## Alternatives considered

### Keep one catalog package and rely on review

- Benefits: Fewer packages and no movement of existing files.
- Costs and risks: The package exposes dozens of unrelated concepts, hides
  capability ownership, and makes dependency violations review-only.
- Reason not selected: The next capabilities would increase coupling faster
  than reviewers could infer the intended boundary.

### Use global domain, ports, services, and adapters layers

- Benefits: Familiar textbook directory names and simple layer diagrams.
- Costs and risks: Related behavior is scattered by technical role, ports lose
  their consumer, and generic packages become dependency magnets.
- Reason not selected: Argus evolves by product capability and needs cohesive
  vertical modules, not horizontal buckets.

### Split capabilities into services now

- Benefits: Network and deployment isolation from the beginning.
- Costs and risks: Premature APIs, distributed failure modes, duplicated
  operations, and slower refactoring before lifecycle evidence exists.
- Reason not selected: ADR-0002 requires evidence before topology splits; Go
  package boundaries provide the needed modularity now.

## Consequences

### Positive

- Package imports expose policy, port, and adapter ownership directly.
- Application services can be tested with small consumer-owned fakes while
  infrastructure remains replaceable.
- Change-impact and selection can become separate hexagons without gaining
  access to catalog SQL or CLI details.
- CI rejects key dependency-direction regressions deterministically.
- Commands become small, auditable composition roots.

### Negative

- Internal import paths change and some shared types require explicit catalog
  qualification.
- Cross-capability workflows need a deliberate orchestrator rather than calls
  through one large package.
- The import test must be maintained as new capabilities and adapters appear.

### Neutral or follow-up

- PostgreSQL remains one adapter package with distinct `Store` and `Migrator`
  types; database-role separation is an operational concern, not a reason to
  duplicate connection primitives.
- Stable catalog errors remain shared until a capability requires meaning that
  is not catalog-wide.
- Future network transports can call the same services without moving domain
  policy or persistence interfaces.

## Compatibility and migration

This is an internal, behavior-preserving refactor. Effect Schema sources,
generated JSON Schema, JSON output, cursor payloads and hashes, SQL migrations,
table layout, canonical fingerprints, environment variables, and command-line
syntax do not change. No data migration or deployment coordination is needed.

## Security and operations

The refactor narrows ambient authority. Only composition roots import the
PostgreSQL adapter, and only the migration command constructs a `Migrator`.
The CLI adapter cannot silently open a different infrastructure dependency.
Existing secret handling, transaction boundaries, timeouts, validation, and
least-privilege database guidance remain unchanged.

## Validation

The decision is verified by:

- the executable hexagonal import-boundary test;
- unit and race tests for domain, services, adapters, and commands;
- unchanged contract compatibility fixtures and output-shape tests;
- PostgreSQL migration-chain, retry, conflict, pagination, and restart tests;
- `make validate`; and
- `make db-validate` against PostgreSQL 17.11.
