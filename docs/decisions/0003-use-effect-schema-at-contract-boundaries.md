# ADR-0003: Use Effect Schema at contract boundaries

- Status: Accepted
- Date: 2026-09-16
- Milestone: M02 - Contracts and identity
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Author external contracts in stable Effect Schema v3 and treat the
  TypeScript source as authoritative.
- Compile deterministic JSON Schema Draft 2020-12 artifacts for validation,
  interoperability, OpenAPI composition, and downstream code generators.
- Keep domain models idiomatic and hand-written; generated wire types are not
  the domain model.
- Add a language binding only when a real producer or consumer needs it.
- Prove structural and semantic compatibility with one fixture corpus.

## Context

Argus needs a repository-authored descriptor before it needs a broad abstract
identity kernel. The descriptor is a real product boundary: repositories
declare components, capabilities, test suites, stable tests, and mappings, and
the Go control plane ingests that declaration at a verified immutable revision.

JSON Schema is a strong interchange and validation format but a weak authoring
environment for composition, refactoring, inferred types, and executable
transforms. Making generated language models the domain model would transfer
wire-format compromises into application code. Generating Go and Python stubs
without real consumers would also create maintenance work and false evidence of
cross-language compatibility.

Effect Schema provides a typed TypeScript authoring model and a built-in JSON
Schema compiler. JSON Schema and OpenAPI tooling can then remain the portable
interchange layer for Go, Rust, Python, Java, and other consumers. TypeScript is
admitted here for a delivered contract capability under ADR-0002; it is not a
second control plane.

## Decision

### Source and generated artifacts

The authoritative source for new external Argus data contracts is Effect
Schema v3 in `contracts/source`. The repository uses an exact stable Effect
version, an exact TypeScript toolchain, and a pnpm lockfile. Effect v4 release
candidates are not used while a stable v3 line is available.

The checked-in JSON Schema Draft 2020-12 document in `contracts/generated` is a
deterministic build artifact. It is the language-neutral validation and
interoperability representation, not a separately edited source. Generation
MUST fail verification when the source and committed artifact differ. Stable
HTTPS schema identifiers MUST resolve locally during validation and MUST NOT
cause a network fetch.

An OpenAPI document MAY reference or incorporate the generated schema when an
HTTP API exists. A generator CLI MAY consume JSON Schema or OpenAPI to create
Rust, Go, Python, Java, or other transport stubs when a real consumer requires
them. Neither OpenAPI nor unused stubs are generated speculatively.

### Runtime boundary and domain conversion

Incoming documents are untrusted. The Go boundary validates raw JSON against
the generated schema before decoding a small hand-written transport DTO. It
then converts that DTO into an idiomatic Go catalog model and enforces semantic
invariants that JSON Schema does not express, including uniqueness,
referential integrity, normalized repository-relative paths, and verified
revision context.

Repository identity is the tuple of provider, host, and provider-assigned
opaque repository identifier. Owner and repository name are mutable display
coordinates. The immutable revision comes from a trusted ingestion context; a
repository-controlled descriptor cannot assert which revision was fetched.
Local descriptor keys are scoped by their containing repository, suite, or
declaration rather than encoded as premature global URNs.

The contract taxonomy may name a test family without implying that Argus can
yet discover, select, execute, or adapt that family. Support policy is a
separate product capability. In particular, unit tests are catalogable but are
not part of the initial selection or automated-adaptation delivery path.

### Compatibility corpus and versioning

One manifest-owned corpus records structural validity and domain validity
separately. Effect Schema tests and Go schema tests consume the same documents.
Go domain tests consume every structurally valid document and verify the
semantic expectation. Unknown fields are rejected.

Each contract carries an explicit major in `apiVersion` and its generated path.
A breaking change creates a parallel major. The repository does not add a
second bundle version or minimum-reader version until an actual independently
released bundle or rolling producer/consumer deployment requires it.

## Alternatives considered

### Hand-author JSON Schema

- Benefits: Language-neutral and directly supported by validators and code
  generators.
- Costs and risks: Weaker refactoring and composition ergonomics, duplicated
  TypeScript types, and verbose authoring.
- Reason not selected: JSON Schema remains the generated interchange artifact,
  while Effect Schema provides a better authoritative authoring model.

### Protocol Buffers as the source

- Benefits: Mature multi-language generators and compatibility conventions.
- Costs and risks: Adds a compiler and field-number lifecycle while repository
  descriptors and GitHub-facing payloads remain JSON-oriented.
- Reason not selected: Human-reviewable JSON and strict local validation are
  more useful for the first boundary.

### Generate every language binding immediately

- Benefits: Demonstrates generator reach and creates ready-made stubs.
- Costs and risks: Adds toolchains, generated churn, and unsupported APIs with
  no real producer or consumer.
- Reason not selected: Consumer-driven generation gives compatibility tests a
  concrete purpose and avoids speculative maintenance.

### Reuse or extract Perfeng contracts now

- Benefits: One contract toolchain across both products.
- Costs and risks: Couples product vocabulary, release ownership, and migration
  cadence before a genuinely shared contract exists.
- Reason not selected: Argus reuses the architectural pattern. Extraction still
  requires the multi-consumer evidence in ADR-0001.

## Consequences

### Positive

- Contract authors get typed composition and inferred TypeScript types.
- JSON Schema remains portable to validators, OpenAPI, and downstream language
  generators.
- The Go catalog remains idiomatic and does not depend on generated domain
  structures.
- The real descriptor and CLI exercise a product boundary rather than a
  synthetic conformance envelope.
- Language workspaces enter the repository only with delivered behavior.

### Negative

- Contract changes require both Node/pnpm and Go tooling.
- JSON Schema compilation can express only the Effect features supported by
  the compiler target; generation tests must catch unsupported constructs.
- Hand-written transport DTOs require a corpus test to prevent drift.
- Semantic invariants remain application code rather than one universal schema.

### Neutral or follow-up

- M03 persists normalized snapshots in PostgreSQL and proves migration chains
  against disposable databases.
- OpenAPI generation begins with an actual HTTP boundary.
- A Python analysis package gets generated or hand-written bindings only with
  its first real workload.
- Argus–Perfeng contracts remain M09 work unless an earlier vertical slice
  proves a shared semantic owner and independent release need.

## Compatibility and migration

This is the first published repository descriptor, so no external migration is
required. A future breaking revision adds `v2` beside `v1`; the old generated
schema and fixtures remain while supported consumers migrate. Compatible
changes still require review of strict-reader rollout because unknown fields
are rejected.

## Security and operations

Consumers MUST bound input size at their transport boundary, validate the
complete document before domain use, and avoid remote schema resolution. The
descriptor contains no credentials or executable instructions. Adapter names
select only server-owned implementations; they are not commands to execute.
Repository coordinates and paths are data and MUST NOT become authorization or
filesystem authority without an explicit trusted mapping.

The pnpm lockfile, Go checksums, vulnerability scans, runtime license checks,
and major-only GitHub Action tags follow the repository dependency policy.

## Validation

The decision is verified by:

- deterministic Effect Schema to JSON Schema generation and drift detection;
- TypeScript formatting, linting, type checking, and fixture tests;
- strict Go validation against the generated schema;
- Go domain conversion over structurally valid fixtures;
- positive, structural-negative, and semantic-negative fixtures;
- a CLI that imports a descriptor with a verified revision and emits a stable
  catalog summary;
- ordinary and race-enabled Go tests; and
- `make validate` on Ubuntu 24.04 and Windows Server 2025.
