# Argus implementation milestones

## TL;DR

- Milestones are outcome-oriented, numbered M01 through M10, and must not be
  renumbered after publication.
- Acceptance ingredients are grouped into 19 substantial delivery slices;
  ingredients are not individual pull-request boundaries.
- Functional API selection is the first product vertical slice.
- UI adaptation follows only after deterministic impact and validation work.
- Predictive selection follows measured shadow-mode baselines.
- Performance execution and analysis remain owned by Perfeng and integrate
  through versioned contracts in M09.

## Milestone index

| Milestone title | Short message |
|:--|:--|
| M01 - Engineering foundation | Make every change repeatable, reviewable, and safe. |
| M02 - Contracts and identity | Give shared identities and evidence metadata a stable cross-language foundation. |
| M03 - Repository catalog | Build the authoritative map of repositories, capabilities, suites, and tests. |
| M04 - Change impact | Turn a pull-request diff into explainable affected capabilities. |
| M05 - Functional API selection | Produce and execute the first cross-repository impacted-test manifest. |
| M06 - Validated adaptation | Repair narrow functional API drift without weakening test intent. |
| M07 - UI test intelligence | Extend mapping and constrained adaptation to browser tests. |
| M08 - Predictive optimization | Reduce feedback cost using calibrated evidence and safe fallbacks. |
| M09 - Perfeng integration | Request and consume trustworthy performance evidence without duplicating Perfeng. |
| M10 - Production readiness | Operate Argus securely, observably, recoverably, and at scale. |

## Sequencing and promotion

This document is the canonical delivery sequence. The
[product definition](../product/product-definition.md) owns scope and safety;
the [architecture overview](../architecture/overview.md) owns logical
boundaries. GitHub owns live milestone and issue state.

The proposal's calendar phases are represented as outcome milestones:

| Product stage | Milestones | Promotion evidence |
|:--|:--|:--|
| Engineering and identity foundation | M01–M03 | Reproducible build, versioned contracts, stable identities, and a queryable design-partner catalog |
| Deterministic functional API selection | M04–M05 | Explainable change impact, stable test discovery, shadow/full-run comparison, and published misses |
| Validated functional API adaptation | M06 | Original failure, minimal repair, repaired success, negative control, and review evidence |
| UI intelligence | M07 | Reliable UI identity/mapping and measured locator-repair precision without assertion weakening |
| Predictive optimization | M08 | Chronological replay and shadow mode beat baselines within an owner-approved missed-failure envelope |
| Performance evidence | M09 | Versioned Argus–Perfeng flow preserves authority, identity, quality, and provenance |
| Production operation | M10 | Security, reliability, recovery, observability, cost, and ownership controls are verified |

Completing implementation tasks is necessary but not sufficient for promotion.
Autonomy MUST advance independently per repository, test family, and adaptation
class. When evidence is below the agreed gate, the capability remains in
recommendation or shadow mode and the milestone records that limitation.

## Delivery slices

A delivery slice is the default issue and pull-request boundary. It groups the
contracts, implementation, persistence, adapters, tests, documentation, and
operational evidence needed to prove one observable outcome. A slice may close
several acceptance ingredients in its milestone. Specialized contracts are
defined with the first feature that consumes them instead of in a speculative
schema-only phase.

| Milestone | Delivery slice | Bundled outcome |
|:--|:--|:--|
| M01 | Foundation governance closeout | Automate dependency updates; enforce local vulnerability and licensing checks; define security ownership and the larger-slice delivery policy. |
| M02 | Repository descriptor and identity boundary | Establish Effect-authored contracts, generated JSON Schema, repository and revision identity, catalog-domain conversion, compatibility fixtures, and an executable ingestion seam. |
| M03 | Catalog persistence and descriptor ingestion | Establish PostgreSQL migrations and persist normalized repository descriptor snapshots. |
| M03 | Design-partner mapping and catalog query | Ingest source/test mappings, detect conflicts and staleness, and expose deterministic versioned reads. |
| M04 | Trusted change ingestion | Verify and deduplicate GitHub deliveries, resolve immutable revisions, bound diffs, and produce normalized ChangeSet values. |
| M04 | Semantic API impact | Discover and compare OpenAPI contracts, map changes to capabilities, persist evidence, and expose explainable affected-capability queries. |
| M05 | Explainable functional API selection | Discover and map stable tests, apply deterministic policy and graph rules, and emit an explained ExecutionManifest. |
| M05 | Selection execution and shadow evaluation | Execute the manifest, ingest results, run the remaining/full control, and report time savings and misses. |
| M06 | Constrained functional API repair | Classify stale tests and generate minimal endpoint or field-rename candidates while rejecting intent-changing edits. |
| M06 | Repair validation and review learning | Reproduce failure, validate repair and negative control, package evidence, open a review PR, and ingest the final outcome. |
| M07 | Browser catalog and selection | Discover Playwright tests, map routes/components, ingest browser evidence, and select deterministically. |
| M07 | Constrained locator repair | Diagnose locator failures, generate and reject candidates, validate repeatedly with a negative control, and open an evidence-backed PR. |
| M08 | Offline predictive evaluation | Build chronological data, baselines, replay, ranking, calibration, and abstention evidence. |
| M08 | Safe budgeted scheduling | Estimate cost and dependencies, schedule diverse stages, execute relevant-now/remaining-later modes, and monitor safety. |
| M09 | Perfeng contract and catalog integration | Agree on versioned messages and synchronize workload, environment, baseline, and capability references. |
| M09 | Performance evidence lifecycle | Dispatch and correlate idempotent requests, consume quality/regression evidence, and demonstrate the end-to-end decision flow. |
| M10 | Security, tenancy, and resource controls | Establish identity, authorization, tenancy, least privilege, and bounded resource/model use. |
| M10 | Durable operations and recovery | Add observability, worker leasing, retries, dead letters, backup, restore, retention, and operator recovery. |
| M10 | Deployment and readiness | Package immutable deployment, rollout and rollback, health and SLO monitoring, threat modeling, and production-readiness evidence. |

The table is a planning baseline, not permission to hide incompatible changes
inside a large diff. A slice is split only when it contains independently
valuable outcomes with materially different risk, ownership, rollout, or
review needs.

## M01 - Engineering foundation

**Message:** Make every change repeatable, reviewable, and safe.

Acceptance ingredients:

- Establish the contributor and engineering harness.
- Harden the harness for autonomous contributors and add baseline open-source
  governance.
- Import the product proposal and split product, architecture, and roadmap
  sources of truth.
- Record the control-plane language and cross-project reuse boundary in
  [ADR-0001](../decisions/0001-use-go-and-evidence-based-cross-project-reuse.md).
- Record the initial repository topology in
  [ADR-0002](../decisions/0002-keep-argus-in-a-single-product-repository.md).
- Scaffold the [Go module](../../go.mod) and a minimal
  [control-plane command](../../cmd/control-plane/main.go) with graceful startup
  and shutdown.
- Add pinned formatting, linting, static analysis, test, race, and vulnerability
  checks through the root [Makefile](../../Makefile) and
  [golangci-lint configuration](../../.golangci.yml).
- Add a cross-platform [CI workflow](../../.github/workflows/validate.yml) that
  runs the same checked-in commands.
- Add the [developer quickstart and supported local environment
  contract](../development/developer-quickstart.md).
- Establish the [dependency update, licensing, and security-reporting
  policy](../development/dependency-policy.md).

Completion means a new contributor can build, verify, and understand the empty
system skeleton using versioned instructions.

## M02 - Contracts and identity

**Message:** Turn a real repository descriptor into validated catalog state.

Acceptance ingredients:

- Author the repository descriptor in Effect Schema v3.
- Generate and verify a deterministic JSON Schema Draft 2020-12 artifact.
- Define stable provider repository identity and trusted immutable revision
  context without premature global identifier syntax.
- Declare components, capabilities, test suites, stable tests, test families,
  and explicit capability mappings.
- Validate strict wire structure before converting into an idiomatic Go catalog
  model.
- Enforce semantic uniqueness, reference, path, and revision invariants through
  a shared positive and negative fixture corpus.
- Deliver a command that validates and normalizes one source repository with a
  separate functional-test repository.

The delivered descriptor boundary is documented in the
[contract workspace](../../contracts/README.md); its representation and
compatibility policy are recorded in
[ADR-0003](../decisions/0003-use-effect-schema-at-contract-boundaries.md).

Completion means the Go control plane can strictly validate a repository-owned
descriptor, bind it to a trusted immutable revision, and produce normalized
catalog state for one source repository and one separate test repository.
OpenAPI documents and additional language stubs are added with their first real
producer or consumer.

## M03 - Repository catalog

**Message:** Build the authoritative map of repositories, capabilities, suites,
and tests.

Acceptance ingredients:

- Select the PostgreSQL migration runner and record its ADR.
- Add the PostgreSQL development/test environment and migration-chain checks.
- Create the repository, revision, and component catalog schema.
- Create the capability and authoritative-contract catalog schema.
- Create the test repository, suite, and stable test identity schema.
- Create the impact-edge, provenance, confidence, and expiry schema.
- Define versioned Capability, TestCatalogEntry, ImpactEdge, and
  mapping-conflict contracts.
- Ingest explicit repository and test mappings from one design partner.
- Expose catalog read APIs with deterministic pagination and versioning.
- Detect and report conflicting or stale mappings without overwriting them.

Completion means one source repository and one separate functional-test
repository are represented with stable, queryable identities.

## M04 - Change impact

**Message:** Turn a pull-request diff into explainable affected capabilities.

Acceptance ingredients:

- Define the normalized ChangeSet contract and compatibility fixtures.
- Verify and normalize GitHub webhook deliveries.
- Store webhook delivery identity and make ingestion idempotent.
- Resolve immutable base and head revisions for a pull request.
- Fetch and bound changed-file and patch metadata.
- Implement OpenAPI document discovery and canonicalization.
- Produce semantic OpenAPI operation and schema changes.
- Normalize source, contract, configuration, and test changes into ChangeSet.
- Map explicit OpenAPI changes to capability IDs.
- Persist impact observations with provenance and expiry.
- Expose an affected-capabilities query with human-readable reasons.

Completion means an API-changing pull request produces a reproducible,
explainable capability impact result.

## M05 - Functional API selection

**Message:** Produce and execute the first cross-repository impacted-test
manifest.

Acceptance ingredients:

- Define the functional API adapter protocol and conformance fixtures.
- Define ExecutionManifest, selection-explanation, normalized TestResult, and
  execution-attempt contracts.
- Discover stable functional API test IDs from the design-partner repository.
- Bind OpenAPI operations and capabilities to functional API tests.
- Implement versioned must-run, pin, exclusion, and fallback policy.
- Select impacted tests using deterministic graph rules.
- Generate an ExecutionManifest with inclusion and omission reasons.
- Add a reference CI integration that consumes the manifest.
- Ingest normalized functional API results and artifacts.
- Add a remaining/full-suite control run and compare selection misses.
- Publish a shadow-mode selection report with time and failure recall.

Completion means a pull request can select and execute an explainable subset
while the full suite remains the authority.

## M06 - Validated adaptation

**Message:** Repair narrow functional API drift without weakening test intent.

Acceptance ingredients:

- Define AdaptationProposal, ValidationEvidence, and ReviewOutcome contracts.
- Classify impacted tests as valid, invalidated, uncovered, or uncertain.
- Detect an authoritative one-to-one endpoint rename.
- Generate a deterministic endpoint-reference patch.
- Detect an authoritative compatible request/response field rename.
- Reject candidate changes that alter assertions or unsupported files.
- Create an isolated original-versus-patched validation run.
- Require failure reproduction, repaired success, and a negative control.
- Package provenance, diff, execution, and invariant checks as validation
  evidence.
- Open a review-first GitHub pull request for a validated candidate.
- Ingest reviewer outcome, reason code, and final edited diff.

Completion means one narrow drift class can produce a small evidence-backed PR
without silently turning a failing test green.

## M07 - UI test intelligence

**Message:** Extend mapping and constrained adaptation to browser tests.

Acceptance ingredients:

- Define the browser-test adapter contract and Playwright conformance fixtures.
- Discover stable Playwright test IDs, projects, tags, and owners.
- Map UI routes and declared components to browser tests.
- Ingest browser results, traces, screenshots, and accessibility snapshots.
- Select impacted browser tests using deterministic route/component mappings.
- Diagnose locator-not-found failures separately from product failures.
- Generate locator candidates from stable role, accessible name, and test ID
  evidence.
- Reject ambiguous, broadened, assertion-changing, or cross-flow candidates.
- Validate locator repair with repeated execution and a negative control.
- Open a review-first UI repair PR with visual and structural evidence.

Completion means Playwright selection and one constrained locator repair class
operate under the same evidence model as functional API tests.

## M08 - Predictive optimization

**Message:** Reduce feedback cost using calibrated evidence and safe fallbacks.

Acceptance ingredients:

- Build the chronological execution/change feature dataset.
- Implement run-all, explicit-impact, recent-failure, duration, and random
  evaluation baselines.
- Create the historical replay and temporal holdout harness.
- Train an interpretable test-value ranking baseline.
- Calibrate failure likelihood by repository and test family.
- Add uncertainty-aware abstention and cold-start fallback.
- Add duration, resource, dependency, and environment estimates.
- Schedule a diverse portfolio under a stage budget.
- Implement relevant-now and remaining-later execution modes.
- Monitor missed failures, calibration drift, cost, and time to first failure.

Completion means predictive selection is promoted from shadow mode only for
repositories where measured risk stays within policy.

## M09 - Perfeng integration

**Message:** Request and consume trustworthy performance evidence without
duplicating Perfeng.

Acceptance ingredients:

- Define and jointly review the versioned Argus–Perfeng contract boundary.
- Define PerformanceEvidenceRequested and compatibility fixtures.
- Define run accepted/rejected and analysis-completed result contracts.
- Define PerformanceWorkloadInvalidated and maintenance-routing semantics.
- Synchronize stable Perfeng workload, environment, and baseline references into
  the Argus catalog.
- Map affected capabilities to candidate Perfeng workload IDs.
- Dispatch idempotent evidence requests with immutable source revisions.
- Correlate cancellation, retries, terminal failure, and late results.
- Consume measurement-quality, SLO, regression, uncertainty, and evidence links
  without reading raw k6 output.
- Demonstrate an end-to-end change-to-Perfeng-evidence decision flow.

Completion means Argus can request and use performance evidence while Perfeng
retains authority over workloads, execution, baselines, statistics, and raw
artifacts.

## M10 - Production readiness

**Message:** Operate Argus securely, observably, recoverably, and at scale.

Acceptance ingredients:

- Define authentication, authorization, repository tenancy, and service
  identity.
- Enforce least-privilege GitHub and CI permissions.
- Add structured logs, traces, metrics, correlation, and decision audit views.
- Add durable worker leasing, bounded retries, dead-letter handling, and
  operator recovery.
- Add rate, concurrency, payload, artifact, and model-budget limits.
- Add backup, restore, retention, deletion, and disaster-recovery verification.
- Package immutable deployment configuration and database migrations.
- Add staged rollout, compatibility checks, health probes, and rollback.
- Add SLOs and alerts for ingestion, decision latency, selection safety, and
  adaptation failure.
- Complete threat modeling, dependency/licensing review, and production
  readiness assessment.
- Evaluate repository-hosted dependency graph and advisory integrations against
  the selected GitHub plan, then enable only the checks with explicit ownership.

Completion means Argus can be deployed with explicit security, reliability,
recovery, cost, and ownership controls.

## Planning rules

- GitHub owns live issue state, assignee, labels, and milestone assignment.
- This document owns intended sequencing and scope boundaries until an issue is
  created.
- Promotion targets are set from design-partner baselines; research or vendor
  results MUST NOT be copied as local acceptance thresholds.
- Every issue names one exact milestone.
- Every issue SHOULD target one delivery slice and MAY close multiple acceptance
  ingredients within that milestone.
- Do not create separate issues merely for a contract, table, adapter, test
  fixture, or documentation file when they are necessary parts of one slice.
- Define specialized contracts with the first consuming capability; do not
  front-load schemas that no executable behavior validates.
- Move an issue between milestones only when its outcome dependency changes;
  update this roadmap in the same planning change.
- Split a slice when it contains independently valuable outcomes with
  materially different security, compatibility, ownership, deployment,
  rollback, or review risk.
- A milestone may complete with deliberately deferred items only when the
  milestone message is still true and the deferral is recorded.
