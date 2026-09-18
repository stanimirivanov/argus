# Argus architecture overview

**Status:** Canonical conceptual architecture
**Decision boundary:** Technology and repository topology require ADRs

## TL;DR

- Argus is a control plane for change understanding, test cataloging, impact
  decisions, execution manifests, constrained maintenance, and evidence.
- Domain policy is independent from SCMs, CI systems, test frameworks, model
  providers, persistence, and Perfeng.
- Deterministic policy and impact evidence precede predictive ranking or model
  assistance.
- Decisions are immutable, explainable records; conflicting and uncertain
  evidence remains visible.
- Argus does not execute or analyze performance workloads directly. Perfeng
  retains that authority behind versioned asynchronous contracts.
- [ADR-0001](../decisions/0001-use-go-and-evidence-based-cross-project-reuse.md)
  selects Go for the operational control plane, permits a later versioned
  Python analysis boundary, and requires evidence before cross-project code
  extraction.
- [ADR-0002](../decisions/0002-keep-argus-in-a-single-product-repository.md)
  keeps Argus-owned components in one product repository and requires explicit
  lifecycle evidence and a migration ADR before a split.
- [ADR-0003](../decisions/0003-use-effect-schema-at-contract-boundaries.md)
  selects Effect Schema as the contract source, generated JSON Schema as the
  portable artifact, and consumer-owned domain conversion.
- [ADR-0004](../decisions/0004-use-postgresql-and-embedded-forward-migrations.md)
  selects PostgreSQL and explicit embedded forward migrations for catalog
  persistence.
- [ADR-0006](../decisions/0006-enforce-capability-oriented-hexagonal-boundaries.md)
  makes each application capability own its ports, confines infrastructure to
  adapters, keeps commands as composition roots, and enforces import direction.
- [ADR-0007](../decisions/0007-ingest-github-changes-as-bounded-immutable-evidence.md)
  binds signed GitHub deliveries to immutable, bounded, durably idempotent
  change evidence.
- Queues and deployment topology remain deferred to ADRs and evidence from
  vertical slices.

## Purpose and authority

This document defines system responsibilities, logical boundaries, information
flow, and non-negotiable architectural constraints. It does not itself select a
database, queue, deployment topology, or model provider. Those durable choices
MUST be recorded in
[architecture decision records](../decisions/README.md). ADR-0001 selects the
control-plane language and cross-project reuse policy; ADR-0002 selects the
initial repository topology and split criteria; ADR-0003 selects the contract
authoring and interoperability boundary; ADR-0004 selects the catalog database
and migration policy; ADR-0006 selects the code-level hexagonal boundaries.

The [product definition](../product/product-definition.md) governs product
behavior and safety. The [implementation milestones](../roadmap/milestones.md)
govern sequencing. An ADR MAY refine this overview but MUST identify and resolve
any contradiction explicitly.

## System context

Argus consumes immutable change and test evidence, produces decisions and
versioned manifests, and observes downstream outcomes. Existing SCM, CI, test
runner, artifact, and performance systems continue to execute their specialist
responsibilities.

~~~mermaid
flowchart LR
    DEV[Developers and reviewers] --> SCM[SCM and pull requests]
    SCM --> ARGUS[Argus control plane]
    CI[CI systems] <--> ARGUS
    REPOS[Source, contract, and test repositories] <--> ARGUS
    ARGUS --> RUNNERS[Functional and UI test runners]
    RUNNERS --> ARGUS
    ARGUS <--> PERFENG[Perfeng performance platform]
    ARGUS --> ARTIFACTS[Evidence and artifact stores]
    ARGUS --> DEV
~~~

Argus owns the decision and its provenance. It SHOULD delegate test execution
to existing CI and framework runners. It MUST NOT treat a runner invocation as
proof that the intended revision, environment, or policy was used; those values
must be pinned and returned as evidence.

## Architectural principles

- **Capability-oriented core.** Domain concepts and policies are organized
  around catalog, impact, selection, maintenance, validation, and evidence—not
  vendor APIs or transport layers.
- **Ports at consuming boundaries.** SCM, CI, persistence, artifact, model, and
  framework adapters implement application-owned contracts.
- **Versioned compatibility.** Published APIs, events, schemas, manifests,
  descriptors, policies, model-output schemas, and stored evidence are explicit
  compatibility boundaries.
- **Immutable identity.** Source revisions, test identities, contract versions,
  decisions, attempts, policies, environments, and artifacts use stable or
  immutable identifiers.
- **Evidence with provenance.** Every discovered relationship records source,
  observation time, confidence, and expiry. Explicit conflicts are retained.
- **Conservative fallback.** Unsupported inputs, cold start, unavailable
  validators, or uncertainty select broader execution or abstention.
- **Replaceable intelligence.** Rules, graph queries, ranking, retrieval, and
  model-assisted generation sit behind versioned inputs and measured outputs.
- **Asynchronous safety.** Webhooks, executions, retries, callbacks, and late
  results are correlated and idempotent; external calls stay outside database
  transactions.
- **Least privilege.** Repository writes, CI dispatch, model access, artifacts,
  and credentials are isolated capabilities rather than ambient authority.

## Logical capabilities

| Capability | Responsibility | Does not own |
|:--|:--|:--|
| Change ingestion | Verify delivery identity, resolve immutable revisions, and normalize source/contract/config/test changes | Product impact policy |
| Repository catalog | Store repositories, components, capabilities, contracts, suites, stable test IDs, owners, environments, and mappings | SCM as source of record |
| Impact analysis | Combine explicit, static, dynamic, historical, and reviewed relationships into explainable affected capabilities/tests | Budget or execution scheduling |
| Policy and selection | Apply must-run/fallback rules, ranking, budgets, dependencies, and stage policy | Framework-specific command construction |
| Manifest publication | Produce versioned execution decisions with inclusion, omission, ordering, and reason evidence | Test execution itself |
| Result ingestion | Normalize attempts, results, classifications, and artifact references | Raw framework artifact ownership |
| Maintenance analysis | Classify validity, coverage gaps, adaptation class, and abstention | Unreviewed repository mutation |
| Candidate generation | Apply deterministic transforms first and bounded model assistance second | Policy authority or self-approval |
| Validation orchestration | Compare original/patched behavior, enforce gates, and package evidence | Redefinition of test intent |
| Collaboration | Create review artifacts when authorized and capture structured human outcomes | Treating merge as proof of correctness |
| Offline evaluation | Replay history, compare baselines, calibrate risk, and monitor drift | Inline override of hard safety policy |

These are logical capabilities, not predetermined services. A capability MUST
NOT become a network boundary merely because it appears as a separate row.

## Core information model

The repository descriptor establishes repository, component, capability, suite,
and test identity at a trusted immutable revision. The impact-evidence boundary
adds versioned relationships with evidence type, producer provenance,
confidence, observation time, and expiry. Later boundaries still need stable
identities and versioning for:

- API operation, UI surface, owner, and execution environment;
- normalized change set and semantic change;
- execution manifest and selection explanation;
- execution attempt, normalized result, failure classification, and artifact;
- adaptation proposal, validation evidence, and review outcome; and
- policy, adapter, model, and decision revisions.

The impact relationship is many-to-many and MUST NOT be inferred only from
folder layout. The catalog merges explicit repository declarations with
discovered static, dynamic, historical, and reviewer-confirmed relationships.
Any precedence for explicit declarations belongs to versioned selection policy;
the catalog preserves every observation, and contradictions become reviewable
conflicts rather than silent overwrites.

### Current catalog code boundaries

The first catalog slice implements these logical boundaries in one Go module;
the paths are package ownership boundaries, not a commitment to future service
topology:

| Path | Owns | Must not own |
|:--|:--|:--|
| `internal/catalog` | Shared repository, revision, snapshot, and test identity vocabulary; canonical ordering; invariants; and stable catalog errors | Use-case orchestration, consumer ports, generated contracts, SQL, or adapters |
| `internal/catalog/snapshot` | Snapshot ingestion/retrieval service and its consumer-owned `Store` port | Transport conversion, SQL, or schema administration |
| `internal/catalog/testquery` | Bounded test query service, keyset cursor policy, and its reader port | JSON output, SQL, or impact policy |
| `internal/catalog/impact` | Immutable evidence ingestion, edge query and temporal/conflict policy, cursors, and consumer-owned ports | Generated DTOs, SQL, or selection thresholds |
| `internal/catalog/adapters/contract/*` | Conversion from versioned generated transports and transport-specific semantic errors | Persistence or application orchestration |
| `internal/catalog/adapters/cli/*` | Catalog and descriptor CLI parsing, local file input, application invocation, and versioned JSON output | Concrete infrastructure selection, SQL, or domain policy |
| `internal/catalog/adapters/postgres` | PostgreSQL transactions, relational mapping, private fingerprint encoding, error classification, and explicit migrations | Public wire formats or application policy another adapter would need |
| `cmd/catalog` | Process lifecycle, environment configuration, and concrete adapter composition | Argument policy, output mapping, cursor policy, or direct SQL orchestration |
| `cmd/descriptor` | Descriptor-command process wiring | Descriptor validation, file parsing, or output mapping |
| `cmd/migrate` | Explicit composition of the privileged migration capability | Runtime catalog reads or writes |

Runtime catalog code depends on the narrow `snapshot.Store` port. The
PostgreSQL `Store` implements it, while the separately opened
`Migrator` owns schema administration. Domain structs intentionally have no
JSON tags: the descriptor DTO, command output DTO, and persisted fingerprint
are distinct compatibility boundaries and evolve independently.

The first catalog query is an application use case behind the
`testquery.TestCatalogReader` port. It owns bounded-page and cursor policy, while
the PostgreSQL adapter owns the keyset SQL and the CLI adapter owns conversion
to `argus.dev/test-catalog-page/v1`. This query boundary is reusable by a later
authenticated network API without moving transport concerns into catalog
policy.

Impact evidence follows the same inward dependency direction. The
`impact.EvidenceStore` port accepts immutable, canonical bundles, while the
`impact.EdgeReader` returns raw observations visible at an explicit time. The
application service—not PostgreSQL or the CLI—derives supported, refuted,
stale, and conflicting states. This keeps future storage adapters and network
transports consistent and prevents a database query from becoming hidden
selection policy. [ADR-0005](../decisions/0005-store-immutable-impact-evidence.md)
defines the persisted meaning and temporal rules.

The import-boundary test under `internal/catalog` enforces the dependency
direction: shared domain code cannot point outward, capability packages cannot
import contracts or infrastructure, and the driving CLI adapter cannot select
PostgreSQL. [ADR-0006](../decisions/0006-enforce-capability-oriented-hexagonal-boundaries.md)
defines this modular-monolith structure and the deliberately rejected generic
layer packages.

### Current change-ingestion boundaries

The M04 ingestion slice applies the same hexagonal rule to change evidence:

| Path | Owns | Must not own |
|:--|:--|:--|
| `internal/change` | Provider-neutral change-set vocabulary, bounds, canonical order, and semantic invariants | Contracts, HTTP, provider clients, SQL, or orchestration |
| `internal/change/ingest` | Verified-delivery workflow plus its consumer-owned resolver and store ports | GitHub payloads, generated DTOs, HTTP status policy, or SQL |
| `internal/change/adapters/github` | Raw-body signature verification, GitHub payload normalization, REST pagination, and pre/post revision consistency | Persistence or impact policy |
| `internal/change/adapters/httpapi` | Bounded webhook HTTP input, provider headers, versioned JSON output, and safe error mapping | Provider resolution, SQL, or domain policy |
| `internal/change/adapters/contract` | Change-domain to `argus.dev/change-set/v1` conversion | Provider or persistence behavior |
| `internal/catalog/adapters/postgres` | Atomic delivery claiming, immutable change/file rows, fingerprints, and reconstruction | Webhook parsing or GitHub calls |
| `cmd/control-plane` | Secret/configuration loading, concrete adapter composition, HTTP lifecycle, and graceful shutdown | Change policy or SQL orchestration |

The service checks durable delivery identity before calling GitHub. The
PostgreSQL port performs the final atomic claim, so simultaneous first attempts
still create one result. Provider calls remain outside database transactions.
The GitHub resolver reads pull-request metadata both before and after file
pagination and rejects revision or file-count movement as stale rather than
publishing mixed evidence. The architecture test enforces inward dependencies
for the new domain and application packages.

## End-to-end decision flow

~~~mermaid
flowchart TD
    A[Verified change event] --> B[Resolve immutable base and head]
    B --> C[Normalize semantic ChangeSet]
    C --> D[Catalog and impact evidence]
    D --> E[Execution decision]
    D --> F[Maintenance decision]
    E --> G[Versioned execution manifest]
    G --> H[Existing CI and test runners]
    H --> I[Normalized results and evidence]
    F --> J{Adaptation class}
    J -->|A| K[Constrained candidate]
    J -->|B| L[Review-first candidate]
    J -->|C or uncertain| M[Issue or abstain]
    K --> N[Isolated validation]
    L --> N
    N --> O[Evidence-backed review artifact]
    I --> P[Outcome and audit store]
    O --> P
    P --> Q[Offline replay, calibration, and drift monitoring]
    Q --> E
    Q --> F
~~~

Every decision receives an immutable identity connecting the triggering change,
selected and omitted tests, applicable policy, evidence, candidate patch,
validation attempts, review outcome, and delayed remaining/full-suite result.
Late results MUST attach to the original decision rather than mutate its
historical inputs.

## Decision and evidence invariants

- A decision MUST include both inclusion and omission reasons.
- An evidence reference MUST identify its producer, schema, immutable source,
  checksum or equivalent integrity mechanism, and retention status.
- Missing evidence MUST be distinguishable from negative evidence.
- Confidence MUST identify what is uncertain and how it was calibrated; it is
  not a universal approval threshold.
- Policy overrides MUST be versioned, scoped, reviewable, and included in the
  decision record.
- Retries MUST remain separate attempts; they MUST NOT erase the original
  failure or infrastructure condition.
- Human edits and reason codes MUST be retained separately from generated
  content.
- A merged patch MUST NOT be labelled correct without corroborating execution
  or delayed outcome evidence.
- Data used for training or evaluation MUST retain license, privacy, temporal,
  and repository-scope constraints.

## Perfeng boundary

Argus and Perfeng are cooperating control planes with independent releases and
different authorities:

| Concern | Argus owns | Perfeng owns |
|:--|:--|:--|
| Change understanding | Cross-family source, contract, configuration, and capability impact | Performance-specific workload validity checks |
| Catalog | References to stable workload, environment, and baseline identities | Authoritative workload definitions, adapters, runtime requirements, and versions |
| Decision | Whether a product change warrants performance evidence, with risk and priority | Whether and how an accepted request can execute safely |
| Execution | Dispatch and lifecycle correlation | k6, Playwright/CDP, Kubernetes, Windows-worker, retry, and cancellation behavior |
| Analysis | Consume a versioned verdict as decision evidence | Measurement quality, SLO evaluation, noise-aware regression, change-point analysis, and diagnosis |
| Baselines | Reference an approved immutable baseline identity | Baseline creation, approval, versioning, and comparison semantics |
| Artifacts | Store decision references and integrity metadata | Raw performance evidence, reports, provenance, and artifact registration |
| Evolution | Flag a workload as potentially invalidated | Validate and apply workload changes under Perfeng policy |

The M09 integration SHOULD use versioned asynchronous request, acceptance or
rejection, analysis-completed, and workload-invalidated contracts. Argus MUST
NOT parse raw k6 output, duplicate performance statistics, create or approve
baselines, or silently edit performance workloads. Perfeng MUST NOT be required
to understand every functional test family to fulfill a performance request.

Argus MUST NOT import or copy Perfeng control-plane internals, share private
tables, or depend on Perfeng deployment layout. Reuse begins with published
contracts, conformance fixtures, and proven design approaches. A shared library
or service requires the semantic, ownership, compatibility, and operational
evidence defined by
[ADR-0001](../decisions/0001-use-go-and-evidence-based-cross-project-reuse.md).

## External adapters

Adapters MAY exist in this repository initially and split only when independent
runtime, ownership, consumer, security, or release-cadence evidence justifies
it. Expected adapter boundaries include:

- SCM events and repository content;
- semantic source and contract analysis;
- functional API and browser test discovery;
- CI dispatch and callback/result ingestion;
- artifact stores and evidence retrieval;
- deterministic transform engines;
- model gateways with structured output validation; and
- Perfeng request/result exchange.

Each adapter MUST normalize untrusted external data before it reaches domain
policy and MUST preserve provider-specific identities needed for idempotency and
diagnosis without leaking provider types into the core.

[ADR-0002](../decisions/0002-keep-argus-in-a-single-product-repository.md)
defines the evidence and migration required before an adapter, contract,
analysis component, or deployment asset moves to another repository or service.

## Security and operational boundaries

- Webhook authenticity, replay protection, and delivery identity are ingress
  concerns and MUST be verified before processing.
- Repository read and write permissions MUST be separate. Candidate creation
  MUST use the least privilege allowed by repository policy.
- Generated or repository-supplied code MUST execute in an isolated environment
  with bounded time, CPU, memory, storage, and network access.
- Model output, retrieved content, repository instructions, test artifacts, and
  tool descriptions MUST be treated as untrusted proposals.
- Secrets and sensitive evidence MUST be redacted before model use and MUST NOT
  be placed in prompts, logs, or training sets without explicit policy.
- Every background operation MUST define idempotency, cancellation, timeout,
  retry classification, terminal failure, and operator recovery.
- The system MUST expose structured decision and attempt telemetry without
  using unbounded-cardinality evidence as metric labels.

## Deliberately deferred decisions

The control-plane language, cross-project reuse boundary, and initial repository
topology are no longer deferred: ADR-0001 selects Go and an evidence-based
extraction policy, while ADR-0002 selects a single Argus product repository and
explicit split triggers. The proposal contained other useful candidates, but
the following are not decisions until their ADR or implementation milestone is
accepted:

| Decision | Candidate direction | Authority |
|:--|:--|:--|
| Durable asynchronous work | CI callbacks plus a queue or database-backed work ownership | ADR when required by first workflow |
| Model provider | Hosted or self-hosted model behind a gateway | Decision after bounded use case and evaluation |
| Deployment packaging | Local composition first; Kubernetes packaging only when runtime exists | M10 or earlier operational need |

No implementation SHOULD introduce one of these foundations merely because it
appeared in the original proposal. The current vertical slice and quality
attributes must justify it.
