# Argus contract workspace

## TL;DR

- Effect Schema v3 is the authoritative contract source.
- Generated JSON Schema Draft 2020-12 is the portable validation and generator
  input; do not edit it by hand.
- The first real contract is a repository descriptor ingested by the Go catalog
  at a separately verified immutable revision.
- The test-catalog page contract exposes stable test identities and capability
  mappings through deterministic keyset pagination.
- Impact-evidence bundles preserve immutable producer observations; impact-edge
  pages expose supported, refuted, stale, and conflicting relationships.
- Change-set v1 records a signed pull-request delivery, immutable base/head
  revisions, and explicitly bounded changed-file and patch evidence.
- Capability-impact v1 records semantic OpenAPI document and operation changes,
  explicit capability mappings, unmapped operations, and partial-analysis
  warnings.
- Structural validity and domain validity are distinct and share one fixture
  corpus.
- Generate language stubs only when a real producer or consumer needs them.

## Delivered boundary

`repository-descriptor/v1` lets one source repository declare:

- its stable provider identity and current display coordinates;
- product capabilities;
- source components and their capability mappings;
- test suites, including suites held in other repositories;
- a broad test-family classification; and
- stable suite-scoped tests and their capability mappings.

The source revision is not part of the repository-controlled document. The
ingestion caller supplies a verified full Git digest. This prevents a document
from claiming that it represents a revision other than the content that Argus
actually fetched.

## Layout and authority

| Path | Responsibility |
|:--|:--|
| `source/repository-descriptor-v1.ts` | Authoritative Effect Schema and inferred TypeScript type. |
| `source/test-catalog-page-v1.ts` | Authoritative versioned test-catalog result contract. |
| `source/impact-evidence-bundle-v1.ts` | Authoritative impact-evidence ingestion contract. |
| `source/impact-edge-page-v1.ts` | Authoritative evaluated impact-edge result contract. |
| `source/change-set-v1.ts` | Authoritative normalized pull-request change contract. |
| `source/capability-impact-v1.ts` | Authoritative semantic OpenAPI capability-impact result. |
| `source/repository-descriptor-v1.test.ts` | Structural fixture tests through Effect Schema. |
| `source/test-catalog-page-v1.test.ts` | Test-catalog result compatibility tests through Effect Schema. |
| `scripts/generate.ts` | Deterministic JSON Schema compiler and drift check. |
| `generated/` | Checked-in JSON Schema Draft 2020-12 artifacts. |
| `fixtures/repository-descriptor/v1/manifest.json` | Shared structural and domain expectations. |
| `fixtures/repository-descriptor/v1/` | Positive and negative compatibility documents. |
| `fixtures/test-catalog-page/v1/` | Positive and negative result-contract documents. |
| `fixtures/impact-evidence-bundle/v1/` | Structural and semantic evidence-ingestion fixtures. |
| `fixtures/impact-edge-page/v1/` | Evaluated relationship and conflict fixtures. |
| `fixtures/change-set/v1/` | Positive and negative normalized-change fixtures. |
| `fixtures/capability-impact/v1/` | Positive and negative semantic-impact fixtures. |
| `repository_descriptor.go` | Go schema-validation boundary and transport DTO. |
| `test_catalog_page.go` | Go test-catalog transport DTO and generated-schema validation boundary. |
| `impact_evidence_bundle.go` | Go evidence-bundle transport and schema-validation boundary. |
| `impact_edge_page.go` | Go evaluated impact-edge transport and schema-validation boundary. |
| `change_set.go` | Go change-set transport and generated-schema validation boundary. |
| `capability_impact.go` | Go capability-impact transport and generated-schema validation boundary. |
| `../internal/catalog/adapters/contract/descriptor/` | Repository-descriptor-to-domain conversion and semantic validation. |
| `../internal/catalog/` | Catalog domain vocabulary, use cases, errors, and persistence port. |
| `../internal/change/` | Provider-neutral change invariants and ingestion use case. |

## Change-set contract

`argus.dev/change-set/v1` binds one authenticated GitHub delivery to a source
repository, pull request, immutable base and head revisions, and observation
time. File changes use normalized kinds and retain additions, deletions, an
optional previous path, and bounded patch evidence.

Missing evidence is explicit. `patchStatus` distinguishes a complete patch,
provider-unavailable text (including binary files), a per-file truncation, and
an exhausted aggregate ingestion budget. `filesTruncated` is true when the
provider reported more files than Argus retains. Consumers MUST NOT interpret
either condition as proof that omitted code was unaffected.

Only the Effect source is edited to change wire structure. `make
generate-contracts` updates the generated JSON Schemas; `make check` fails if
the checked-in artifacts are stale.

## Identity and scope

A repository identity is `(provider, host, providerRepositoryId)`. The
provider-assigned identifier is opaque and remains stable across ordinary
renames or ownership transfers. `owner` and `name` are useful display
coordinates, not identity or authorization.

Capability and component keys are scoped to the source repository. Suite keys
are scoped to their test repository, and test keys are scoped to their suite.
The contract intentionally avoids encoding those scopes into speculative
global URNs. Database keys and public resource names can be added when their
real lookup and tenancy requirements exist.

The family vocabulary is descriptive, not an implementation claim:

| Family | Catalog now | Initial execution/selection | Initial automatic adaptation |
|:--|:--:|:--:|:--:|
| Unit | Yes | No | No |
| Component, contract, integration | Yes | Later | No committed scope |
| Functional API | Yes | Planned first | Narrow validated repairs planned |
| Functional UI | Yes | After API | Narrow locator repairs planned |
| End-to-end | Yes | Later | Review-first at most |
| Performance | Yes | Delegated to Perfeng | Delegated to Perfeng |
| Security, resilience, other | Yes | Policy-dependent later | No committed scope |

Cataloging a family therefore does not authorize Argus to execute or modify it.

## Test catalog query contract

`argus.dev/test-catalog-page/v1` returns tests from one immutable repository
descriptor snapshot. Stable test identity is the combination of test repository
provider identity, suite key, and test key. Owner, repository name, test name,
family, adapter, and capability names are descriptive metadata from that
snapshot rather than identity.

Pages use the following canonical ordering:

1. test repository provider;
2. test repository host;
3. provider repository ID;
4. suite key; and
5. test key.

Text ordering uses PostgreSQL's `C` collation so results do not depend on the
database cluster locale. A non-null `nextCursor` is an opaque continuation
token bound to the source snapshot, capability filter, and cursor contract
version. Consumers must not decode, modify, persist as a permanent identifier,
or reuse it with a different query.

An empty `items` array is a successful result when the snapshot exists but no
test matches the optional capability filter. A missing snapshot remains a
distinct not-found outcome before a page document is produced.

## Impact evidence and edge contracts

`argus.dev/impact-evidence-bundle/v1` carries one producer repository revision
and adapter's observations for one immutable catalog snapshot. Each observation
has a stable local key, capability and test identity, support or refutation
assertion, evidence method, confidence in basis points, and rationale. The
bundle separates producer event time, optional expiry, and database ingestion
time. An expiry must be later than the observation; `null` means the producer
declared no expiry.

`argus.dev/impact-edge-page/v1` evaluates all observations visible at an
explicit UTC instant:

| Status | Meaning |
|:--|:--|
| `supported` | At least one active supporting observation and no active refutation. |
| `refuted` | At least one active refutation and no active support. |
| `stale` | Evidence is visible, but every observation has expired. |
| `conflicting` | Active evidence both supports and refutes the same relation. |

Missing evidence is not negative evidence. Confidence belongs to its producer
and is not aggregated by the catalog. A conflict returns the individual
observations and active assertion counts without selecting a winner. Pages are
ordered by capability key followed by stable test identity. Their opaque cursor
is bound to the snapshot, optional capability filter, and evaluation instant.

The immutable storage and evaluation decision is recorded in
[ADR-0005](../docs/decisions/0005-store-immutable-impact-evidence.md).

## Structural and domain validation

Effect Schema and generated JSON Schema enforce wire structure, required
fields, limits, enumerations, and unknown-field rejection. Go then enforces
semantic invariants:

- keys are unique in their declared scope;
- one stable repository identity has one owner/name coordinate pair per
  descriptor;
- component and test capability references resolve;
- repeated capability references are rejected;
- component roots are normalized repository-relative paths; and
- the supplied immutable revision is a normalized full Git SHA-1 or SHA-256;
- evidence observation keys and edges are unique within a bundle;
- producer observation and expiry times are ordered; and
- every persisted evidence edge references a capability and test in its target
  snapshot.

The manifest records `schemaValid` and `domainValid` independently. Any new
semantic rule requires a representative failing fixture. Go transport tests and
Effect tests consume the same structural expectations; Go catalog tests consume
every structurally valid fixture.

## Generation and downstream languages

Install the versions in `.node-version`, `package.json`, `pnpm-lock.yaml`, and
`go.mod`, then run:

~~~sh
pnpm install --frozen-lockfile
make generate-contracts
make fmt
make check
make test
~~~

The generated JSON Schema may be referenced by a future OpenAPI document or
passed to a generator for Rust, Go, Python, Java, or another real consumer.
Generated stubs are transport conveniences. They do not replace consumer-owned
domain models or semantic validation.

## Example

Validate the positive fixture and bind it to an immutable revision:

~~~sh
go run ./cmd/descriptor \
  -revision 0123456789abcdef0123456789abcdef01234567 \
  contracts/fixtures/repository-descriptor/v1/valid/source-and-test-repositories.json
~~~

The command emits a deterministic summary only after structural and domain
validation succeeds. It is an executable ingestion seam for M03 persistence,
not a replacement for the future control-plane API.

The representation decision and compatibility rules are recorded in
[ADR-0003](../docs/decisions/0003-use-effect-schema-at-contract-boundaries.md).
