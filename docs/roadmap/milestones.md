# Argus implementation milestones

## TL;DR

- Milestones are outcome-oriented, numbered M01 through M10, and must not be
  renumbered after publication.
- Every listed work item is intended to fit one independently reviewable pull
  request.
- Functional API selection is the first product vertical slice.
- UI adaptation follows only after deterministic impact and validation work.
- Predictive selection follows measured shadow-mode baselines.
- Performance execution and analysis remain owned by Perfeng and integrate
  through versioned contracts in M09.

## Milestone index

| Milestone title | Short message |
|:--|:--|
| M01 - Engineering foundation | Make every change repeatable, reviewable, and safe. |
| M02 - Contracts and identity | Give every change, capability, test, decision, and result a stable language. |
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

## M01 - Engineering foundation

**Message:** Make every change repeatable, reviewable, and safe.

Work items:

- Establish the contributor and engineering harness.
- Harden the harness for autonomous contributors and add baseline open-source
  governance.
- Import the product proposal and split product, architecture, and roadmap
  sources of truth.
- Record the control-plane language and component-boundary ADR.
- Record the initial repository topology ADR.
- Scaffold the Go module and a minimal control-plane command with graceful
  startup and shutdown.
- Add pinned formatting, linting, static analysis, test, race, and vulnerability
  checks.
- Add a cross-platform CI workflow that runs the same checked-in commands.
- Add the developer quickstart and supported local environment contract.
- Establish dependency update, licensing, and security-reporting policy.

Completion means a new contributor can build, verify, and understand the empty
system skeleton using versioned instructions.

## M02 - Contracts and identity

**Message:** Give every change, capability, test, decision, and result a stable
language.

Work items:

- Create the versioned contract workspace and compatibility-test harness.
- Define repository, revision, component, capability, suite, and stable test
  identifiers.
- Define provenance, confidence, observation time, and expiry values.
- Define the normalized ChangeSet contract.
- Define Capability and TestCatalogEntry contracts.
- Define ImpactEdge and mapping-conflict contracts.
- Define ExecutionManifest and selection-explanation contracts.
- Define normalized TestResult and execution-attempt contracts.
- Define AdaptationProposal, ValidationEvidence, and ReviewOutcome contracts.
- Generate Go and Python bindings reproducibly and verify regeneration.

Completion means components can exchange versioned fixtures without depending
on one another's internal types.

## M03 - Repository catalog

**Message:** Build the authoritative map of repositories, capabilities, suites,
and tests.

Work items:

- Select the PostgreSQL migration runner and record its ADR.
- Add the PostgreSQL development/test environment and migration-chain checks.
- Create the repository, revision, and component catalog schema.
- Create the capability and authoritative-contract catalog schema.
- Create the test repository, suite, and stable test identity schema.
- Create the impact-edge, provenance, confidence, and expiry schema.
- Define and validate the repository-local Argus descriptor.
- Ingest explicit repository and test mappings from one design partner.
- Expose catalog read APIs with deterministic pagination and versioning.
- Detect and report conflicting or stale mappings without overwriting them.

Completion means one source repository and one separate functional-test
repository are represented with stable, queryable identities.

## M04 - Change impact

**Message:** Turn a pull-request diff into explainable affected capabilities.

Work items:

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

Work items:

- Define the functional API adapter protocol and conformance fixtures.
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

Work items:

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

Work items:

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

Work items:

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

Work items:

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

Work items:

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

Completion means Argus can be deployed with explicit security, reliability,
recovery, cost, and ownership controls.

## Planning rules

- GitHub owns live issue state, assignee, labels, and milestone assignment.
- This document owns intended sequencing and scope boundaries until an issue is
  created.
- Promotion targets are set from design-partner baselines; research or vendor
  results MUST NOT be copied as local acceptance thresholds.
- Every issue names one exact milestone.
- Move an issue between milestones only when its outcome dependency changes;
  update this roadmap in the same planning change.
- Split any item discovered to require multiple independently valuable or risky
  changes before implementation begins.
- A milestone may complete with deliberately deferred items only when the
  milestone message is still true and the deferral is recorded.
