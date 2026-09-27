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

### Current change-impact boundaries

The M04 change-impact slice applies the same hexagonal rule to change evidence:

| Path | Owns | Must not own |
|:--|:--|:--|
| `internal/change` | Provider-neutral change-set vocabulary, bounds, canonical order, and semantic invariants | Contracts, HTTP, provider clients, SQL, or orchestration |
| `internal/change/ingest` | Verified-delivery workflow plus its consumer-owned resolver and store ports | GitHub payloads, generated DTOs, HTTP status policy, or SQL |
| `internal/change/impact` | Semantic analyzer and immutable impact-store ports plus assessment retry policy | OpenAPI parser types, GitHub calls, contracts, or SQL |
| `internal/change/workflow` | Ordering of trusted ingestion and durable impact assessment | Provider, parser, contract, or persistence details |
| `internal/change/adapters/github` | Raw-body signature verification, GitHub payload normalization, REST pagination, and pre/post revision consistency | Persistence or impact policy |
| `internal/change/adapters/openapi` | Bounded OpenAPI 3 parsing, semantic comparison, operation fingerprints, and explicit capability mapping | Webhook trust, persistence, or test-selection policy |
| `internal/change/adapters/httpapi` | Bounded webhook HTTP input, provider headers, versioned JSON output, and safe error mapping | Provider resolution, SQL, or domain policy |
| `internal/change/adapters/contract` | Change-domain to `argus.dev/change-set/v1` conversion | Provider or persistence behavior |
| `internal/catalog/adapters/postgres` | Atomic delivery and impact claims, typed immutable evidence rows, fingerprints, and reconstruction | Webhook parsing, GitHub calls, or OpenAPI semantics |
| `cmd/control-plane` | Secret/configuration loading, concrete adapter composition, HTTP lifecycle, and graceful shutdown | Change policy or SQL orchestration |

The service checks durable delivery identity before calling GitHub. The
PostgreSQL port performs the final atomic claim, so simultaneous first attempts
still create one result. Provider calls remain outside database transactions.
The GitHub resolver reads pull-request metadata both before and after file
pagination and rejects revision or file-count movement as stale rather than
publishing mixed evidence. The workflow acknowledges a webhook only after its
impact assessment is also durable. Exact assessment retries avoid document I/O.
Changed operations map to capabilities only through explicit
`x-argus-capabilities` declarations; unmapped operations and partial analysis
remain visible so M05 can broaden or abstain. The architecture test enforces
inward dependencies for the domain, application, workflow, and driving-adapter
packages.

### Current functional API selection boundaries

The first M05 slice consumes M03 catalog and M04 impact through selection-owned
ports:

| Path | Owns | Must not own |
|:--|:--|:--|
| `internal/selection` | Manifest vocabulary, outcomes, reasons, canonical order, bounds, and invariants | SQL, generated contracts, catalog queries, or execution |
| `internal/selection/functionalapi` | Deterministic targeted/fallback policy and coverage-gap derivation | PostgreSQL, JSON, CLI arguments, or framework commands |
| `internal/selection/adapters/catalogreader` | Bounded traversal and projection of immutable catalog pages | Selection policy |
| `internal/selection/adapters/contract` | Domain-to-execution-manifest v1 conversion | Policy or persistence |
| `internal/selection/adapters/cli/selectioncli` | Local arguments and JSON output through injected ports | Concrete infrastructure |
| `cmd/select` | PostgreSQL composition, process lifecycle, and environment configuration | Selection or serialization policy |

The selector uses the approved base-revision catalog. Complete, fully mapped
impact enables targeted early execution. Partial, empty, or unmapped impact
requires every functional API candidate. Tests omitted from the early stage
retain a mandatory full-suite path, and affected capabilities without a mapped
test are reported as coverage gaps. The manifest is deterministic output, not
release authority.

### Current functional API execution boundaries

The second M05 slice consumes the manifest through an execution-owned process
protocol:

| Path | Owns | Must not own |
|:--|:--|:--|
| `internal/execution` | Attempt vocabulary, result semantics, bounds, canonical order, and evidence invariants | Process execution, JSON, CI configuration, or persistence |
| `internal/execution/functionalapi` | Stage planning, adapter port, exact request/result correlation, and attempt assembly | Framework commands, generated contracts, or artifact upload |
| `internal/execution/planning` | Deterministic repository/adapter grouping, exact reviewed binding coverage, and flat selected/full-suite job planning | Contracts, process execution, SQL, CI syntax, commands, or credentials |
| `internal/execution/attempts` | Validated immutable-attempt ingestion use case and consumer-owned persistence port | SQL, JSON, or artifact upload |
| `internal/execution/shadow` | Explicit pair and complete-plan compatibility checks, aggregate duration/failure recall, and miss classification including full-only groups | Attempt lookup SQL, implicit latest selection, report persistence, or release policy |
| `internal/execution/adapters/contract` | Adapter request/result and execution-attempt v1 conversion | Execution policy or process lifecycle |
| `internal/execution/adapters/processadapter` | Bounded stdin/stdout exchange with an explicit executable | Test selection or command discovery |
| `internal/execution/adapters/cli/executioncli` | Manifest input, explicit group arguments, timeout, and normalized output | Shell evaluation or concrete adapter construction |
| `internal/execution/adapters/cli/planningcli` | Bounded manifest/binding input and execution-plan JSON output | Checkout, command mapping, process execution, or persistence |
| `internal/execution/adapters/cli/evidencecli` | Attempt-document input, explicit comparison IDs, and JSON output through injected ports | PostgreSQL or pairing policy |
| `internal/catalog/adapters/postgres` | Atomic attempt/result/reference persistence, exact-retry detection, and repeatable-read reconstruction | Shadow comparison policy |
| `cmd/run-functional-api` | Process-adapter composition and operating-system streams | Execution or contract policy |
| `cmd/plan-functional-api` | Framework-free planning command composition | CI-provider APIs or adapter execution |
| `cmd/execution-evidence` | PostgreSQL composition, lifecycle, and database configuration | Ingestion or comparison policy |

The manifest chooses tests, while reviewed CI configuration chooses the
adapter executable. Every attempt names an immutable test revision and the
SHA-256 of the canonical manifest. The adapter cannot add tests, omit requested
results, or declare its own aggregate outcome. Non-passing normalized evidence
is emitted before the reference command fails the CI step.

Attempt IDs are global immutable idempotency keys. The PostgreSQL adapter
stores normalized child evidence atomically and distinguishes exact retries
from conflicting ID reuse. Artifact rows register metadata and checksums; they
do not upload or guarantee external bytes. Shadow comparison loads two named
attempts, rejects mismatched provenance or an incomplete control set, and
reports both aggregate recall and each missed failure. It never chooses a
control by recency. See [ADR-0011](../decisions/0011-store-immutable-attempts-and-compare-explicit-shadow-pairs.md).

### Current functional API adaptation boundaries

The first M06 slice adds a proposal-only adaptation capability without giving
the control plane framework-specific source knowledge or mutation authority:

| Path | Owns | Must not own |
|:--|:--|:--|
| `internal/adaptation` | Proposal, validation, and review-publication vocabulary; endpoint rename and edit invariants; canonical evidence; stable errors; and policy versions | Generated contracts, process execution, filesystem access, or provider APIs |
| `internal/adaptation/functionalapi` | Unique endpoint-rename proof, adapter port, proposal identity, result correlation, and constrained edit policy | Framework parsers, JSON, commands, source mutation, or review APIs |
| `internal/adaptation/validation` | Three-phase outcome policy, exact span materialization, negative-control derivation, runner/workspace ports, restoration checks, and evidence assembly | Filesystem APIs, framework commands, JSON, persistence, or review APIs |
| `internal/adaptation/review` | Proposal/evidence correlation, immutable-source reconstruction, deterministic review identity, review content, and the consumer-owned provider gateway | GitHub HTTP, credentials, JSON, merge policy, or reviewer outcomes |
| `internal/adaptation/adapters/contract` | Adaptation, validation, and review request/result/evidence conversion | Rename inference, validation policy, source mutation, or provider calls |
| `internal/adaptation/adapters/processadapter` | Bounded no-shell request/result exchange with an explicit executable | Command discovery, source mutation, or adaptation policy |
| `internal/adaptation/adapters/fileworkspace` | Root containment, regular-file access, preimage-guarded writes, and restoration in a disposable checkout | Validation outcome policy or test execution |
| `internal/adaptation/adapters/validationprocessadapter` | Bounded no-shell phase execution in the disposable checkout | Patch selection, source writes, or outcome policy |
| `internal/adaptation/adapters/githubreview` | Stable repository-identity verification and idempotent GitHub branch, single-file commit, and draft-PR publication | Repair eligibility, candidate construction, merge, or review interpretation |
| `internal/adaptation/adapters/cli/proposalcli` | Bounded impact/manifest input, exact test selection, provenance correlation, timeout, and proposal JSON output | Concrete process construction or framework parsing |
| `internal/adaptation/adapters/cli/validationcli` | Bounded proposal input, explicit disposable root, total timeout, and validation-evidence JSON output | Concrete workspace/process construction or validation policy |
| `internal/adaptation/adapters/cli/reviewcli` | Bounded proposal/evidence input, secret environment configuration, timeout, and review-publication JSON output | Candidate policy, GitHub HTTP, or merge authority |
| `cmd/propose-functional-api-repair` | Process-adapter composition and operating-system streams | Adaptation or contract policy |
| `cmd/validate-functional-api-repair` | Guarded workspace and validation-process composition | Validation, patch, or contract policy |
| `cmd/open-functional-api-repair-pr` | GitHub review-adapter composition and operating-system streams | Review eligibility, candidate construction, or contract policy |

Argus requires complete semantic impact and one contract-declared operation
identity moving from an old path to a new path. Repository-owned adapters map
stable test IDs to native syntax and may return only one byte-addressed
`request-target` edit. The application revalidates the old/new strings and all
identities before emitting `PATCH_AND_VALIDATE`. Proposal generation never
writes to the test checkout and therefore cannot silently repair, commit, or
merge a test. [ADR-0014](../decisions/0014-generate-endpoint-repair-proposals-through-reviewed-adapters.md)
defines the evidence threshold and adapter boundary.

Validation verifies the source preimage and runs the exact test against
unchanged, candidate, and deterministic negative-control bytes. The application
owns the required `failed → passed → failed` sequence and restores the original
file after each modified execution. Adapters only translate stable test IDs to
framework invocation. Successful evidence retains every executed source digest
and the final restored digest. [ADR-0015](../decisions/0015-validate-repairs-with-a-restored-negative-control-workspace.md)
defines the workspace and negative-control policy.

Review publication reloads the source at the proposal's immutable test
revision, verifies the complete-file preimage, reconstructs the exact candidate,
and checks its digest against validation evidence before any external write.
The GitHub adapter verifies stable repository identity, uses a deterministic
branch, and treats an exact retry as the same operation. A branch, file, or
pull request that diverges from the validated candidate is a conflict rather
than an overwrite. Publication creates only an open draft and returns
`argus.dev/adaptation-review/v1`; it never marks the review ready or merges it.
[ADR-0016](../decisions/0016-publish-validated-repairs-as-idempotent-draft-pull-requests.md)
defines the external-write and recovery boundary.

For heterogeneous manifests, the planner groups by stable test-repository
identity and adapter. Reviewed bindings must cover that group set exactly and
provide matching coordinates, a stable group key, and an immutable revision.
The plan creates selected jobs only for non-empty early subsets and a
full-suite job for every group. It carries no command or credential; reviewed
CI maps adapter IDs to literal commands. See
[ADR-0012](../decisions/0012-plan-execution-from-reviewed-repository-bindings.md)
and the [reference GitHub Actions handoff](../integrations/github-actions-functional-api.md).

Complete-plan shadow evaluation binds every planned group to explicit immutable
attempt IDs and rejects missing, additional, reused, or provenance-mismatched
evidence. Selected/full groups reuse pairwise semantics. A full-only group has
zero selected work, and each failing control test is a `not-selected` miss in
aggregate recall. Reports record the canonical plan digest and sum normalized
per-test durations rather than parallel workflow wall time. See
[ADR-0013](../decisions/0013-evaluate-selection-across-complete-execution-plans.md).

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
