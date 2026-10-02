# Argus contract workspace

## TL;DR

- Effect Schema v3 is the authoritative contract source.
- Generated JSON Schema Draft 2020-12 is the portable validation and generator
  input; do not edit it by hand.
- Go wire DTOs and validators are product-internal, not a public Go SDK;
  binaries embed the same checked-in portable schemas.
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
- Execution-manifest v1 records deterministic functional API inclusion and
  omission decisions, immutable input provenance, remaining-suite obligations,
  and uncovered capabilities.
- Execution-manifest v2 records capability-based `functional-ui` decisions for
  OpenAPI-only changes; unknown UI impact conservatively requires every test.
- Functional API adapter request/result v1 and execution-attempt v1 provide a
  bounded framework-neutral CI execution boundary with exact result
  correlation.
- Functional API execution bindings v1 map heterogeneous manifest groups to
  reviewed immutable revisions; execution-plan v1 provides a deterministic
  flat CI matrix with mandatory full-suite jobs.
- Selection-shadow-report v1 compares two explicit compatible attempts and
  exposes duration reduction, failure recall, and individual misses.
- Execution-plan attempt bindings and selection-plan shadow-report v1 evaluate
  every heterogeneous group, including full-only groups, without implicit
  attempt discovery.
- Adaptation-review v1 records the open draft GitHub pull request created from
  one correlated proposal and successful validation proof.
- Review-outcome v1 records a terminal provider-derived disposition, explicit
  reviewer reason, and complete bounded edits after the generated commit.
  Its published v1 identity excludes retry observation time and is verified
  during durable ingestion; persistence separately fingerprints the complete
  provenance.
- Validation-rejection v1 records a trustworthy completed validation prefix
  when a policy gate disproves a candidate; it is never successful proof.
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
| `source/execution-manifest-v1.ts` | Authoritative functional API execution-manifest result. |
| `source/execution-manifest-v2.ts` | Authoritative functional UI capability-selection result. |
| `source/functional-api-execution-v1.ts` | Authoritative adapter request/result and normalized execution-attempt contracts. |
| `source/functional-api-execution-plan-v1.ts` | Authoritative reviewed execution-bindings and generated execution-plan contracts. |
| `source/selection-shadow-report-v1.ts` | Authoritative selected-versus-full-suite shadow report contract. |
| `source/selection-plan-shadow-report-v1.ts` | Authoritative complete-plan attempt-binding and aggregate shadow-report contracts. |
| `source/adaptation-v1.ts` | Authoritative repair proposal and framework-adapter contracts. |
| `source/adaptation-validation-v1.ts` | Authoritative isolated validation request, result, and evidence contracts. |
| `source/adaptation-review-v1.ts` | Authoritative review-first publication contract. |
| `source/review-outcome-v1.ts` | Authoritative terminal review-outcome and reviewer-edit contract. |
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
| `fixtures/execution-manifest/v1/` | Positive and negative selection-manifest fixtures. |
| `fixtures/execution-manifest/v2/` | Browser capability-selection compatibility fixture. |
| `fixtures/functional-api-adapter-request/v1/` | Adapter request conformance fixtures. |
| `fixtures/functional-api-adapter-result/v1/` | Adapter result conformance fixtures. |
| `fixtures/execution-attempt/v1/` | Normalized attempt conformance fixtures. |
| `fixtures/functional-api-execution-bindings/v1/` | Reviewed heterogeneous group-binding compatibility fixtures. |
| `fixtures/functional-api-execution-plan/v1/` | Deterministic CI plan compatibility fixtures. |
| `fixtures/selection-shadow-report/v1/` | Shadow report compatibility fixtures. |
| `fixtures/execution-plan-attempt-bindings/v1/` | Explicit plan-to-attempt binding fixtures. |
| `fixtures/selection-plan-shadow-report/v1/` | Complete-plan aggregate shadow-report fixtures. |
| `fixtures/review-outcome/v1/` | Terminal review-outcome compatibility fixtures. |
| `fixtures/validation-rejection/v1/` | Trustworthy unsuccessful-validation compatibility fixtures. |
| `assets.go` | Embeds the portable generated JSON Schemas for deployed Go binaries; exposes no DTOs. |
| `../internal/contracts/` | Product-internal Go wire DTOs, schema validators, and tests against the shared fixture corpus. |
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
| Functional API | Yes | Selection and execution evidence | Narrow validated repairs |
| Functional UI | Yes | Selection for fully covered API changes only; no execution yet | Narrow locator repairs planned |
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

## Execution manifest contract

`argus.dev/execution-manifest/v1` records one decision for every functional
API candidate in the approved base-revision catalog. Each decision contains the
stable test identity, outcome, remaining execution obligation, and
machine-readable reasons. The manifest also identifies its change assessment,
catalog snapshot, analyzer, and policy versions.

`targeted` mode requires tests mapped to affected capabilities and marks other
candidates `SKIP_FOR_NOW` with a required later full-suite path. `fallback`
mode requires every candidate when impact is partial, empty, or unmapped.
`uncoveredCapabilities` identifies affected behavior without a mapped
functional API test. The manifest is an explainable selection result; it does
not claim that execution occurred or that an early subset is release authority.

## Functional API execution contracts

`argus.dev/functional-api-adapter-request/v1` supplies one exact, bounded test
set to a CI-local adapter. It binds the request to canonical manifest bytes, an
immutable test-repository revision, an explicit selected or full-suite stage,
and the expected adapter identity.

`argus.dev/functional-api-adapter-result/v1` is untrusted adapter output. Argus
requires one normalized result for every requested test and rejects duplicates,
missing results, additional results, identity mismatches, invalid time ranges,
or unbounded evidence. `argus.dev/execution-attempt/v1` is emitted only after
that correlation succeeds. The attempt outcome is derived from per-test
outcomes rather than accepted from the adapter.

The contracts transport artifact references, not artifact bytes or proof that
an object was uploaded. Attempt ingestion records immutable normalized evidence
and artifact metadata; external object upload and checksum verification remain
the CI or artifact-store owner's responsibility.

`argus.dev/functional-api-execution-bindings/v1` is reviewed configuration for
heterogeneous execution. Each binding gives one repository/adapter group a
stable local group key and immutable test revision. It contains no executable,
secret, runner label, or environment. Planning requires the binding set to
match the manifest groups exactly.

`argus.dev/functional-api-execution-plan/v1` binds a flat, deterministic job
array to the SHA-256 of the canonical manifest. A group receives a `selected`
job only when at least one test is required and always receives a `full-suite`
job. CI can use that array directly as a matrix while retaining authority over
checkouts, adapter commands, credentials, isolation, and release gates.

`argus.dev/execution-plan-attempt-bindings/v1` explicitly names the immutable
attempts produced for every plan group. `selectedAttemptId` is `null` only when
the plan has no selected job for that group; `fullSuiteAttemptId` is always
required. Semantic validation rejects incomplete or additional groups, reused
attempt IDs, and selected-attempt presence that disagrees with the plan.

`argus.dev/selection-plan-shadow-report/v1` records the canonical execution
plan digest and aggregates every validated group. Each loaded attempt must
match the plan's manifest, repository, revision, adapter, stage, and exact test
count. Full-suite failures in a full-only group are `not-selected` misses and
remain in the aggregate recall denominator. Duration fields sum normalized
per-test durations; they do not represent parallel CI wall-clock time.

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
