# ADR-0002: Keep Argus in a single product repository

- Status: Accepted
- Date: 2026-09-16
- Milestone: M01 - Engineering foundation
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Keep all Argus-owned components in this repository initially.
- Use one root Go module and one control-plane deployable until observable
  runtime or ownership needs justify another boundary.
- Organize implementation by domain capability, with adapters depending inward;
  do not mirror a future service topology in empty packages.
- Add contracts, migrations, analysis, adapters, and deployment assets here
  only when a delivered capability needs them.
- Keep Perfeng as an independent product family connected through pinned,
  versioned contracts rather than Git or source-layout coupling.
- Split a component only after explicit release, security, scaling, runtime,
  ownership, distribution, or proven-reuse evidence and a migration ADR.

## Context

Argus is at engineering-foundation stage. It has product, architecture, and
delivery boundaries, but no executable component, public contract, database,
or deployment. The first useful path crosses change ingestion, catalog and
impact policy, test selection, manifest publication, result ingestion, and
evidence. Splitting these capabilities into repositories or services before
that path exists would require versioning, release ordering, local composition,
CI, compatibility, observability, and failure handling without proving that the
boundaries are independently valuable.

[ADR-0001](0001-use-go-and-evidence-based-cross-project-reuse.md)
selects Go for the operational control plane, permits later Python analysis and
TypeScript adapter boundaries, and prevents direct reuse of Perfeng internals.
It does not decide where Argus-owned source, contracts, tools, migrations, or
deployment material live.

Perfeng provides useful counter-evidence as well as useful patterns. Its
polyrepo separates mature independently versioned contracts, control-plane,
analysis, runners, Kubernetes packaging, and environment composition. Those
repositories now have distinct responsibilities and toolchains, but recreating
the same topology for a new product would copy its current outcome without
copying the implementation history and operational evidence that justified it.

The topology must keep early vertical slices inexpensive while preserving a
disciplined path to independent components when their lifecycle actually
diverges.

## Decision

### Repository and deployable boundary

The `argus` repository is the initial system of record for all Argus-owned
production source, tests, language-neutral contracts, generated bindings,
database migrations, development tooling, documentation, and deployment
assets. This is a single-product monorepo; it is not a repository for Perfeng
source or a general testing-platform foundation.

The initial control plane uses one root Go module and one production deployable.
Multiple commands MAY share the module when they are operational entry points
for the same release, such as a server and an explicitly invoked migration or
validation command. A command MUST NOT be created merely to imply a service
boundary.

The repository MAY later contain a Python or TypeScript workspace when a
delivered capability requires its native ecosystem. A second language does not
by itself require a second Git repository, service, version, or deployment.
Each workspace MUST have pinned tooling, deterministic checks, explicit inputs
and outputs, and a clear owner within the Argus release.

Repository membership does not grant access to internal state. Logical modules
own their writes and expose application capabilities or versioned events to
other modules. Transport objects, persisted records, and domain values remain
distinct even while they are built and released together.

### Capability-oriented source organization

Production code will be organized around capabilities such as ingestion,
catalog, impact, selection, maintenance, validation, and evidence. Dependencies
MUST point from composition and adapters toward application and domain policy.
A global directory tree that groups every capability into generic `domain`,
`services`, `repositories`, or `utils` layers is not the default.

The first Go scaffold will add only the packages needed for process lifecycle
and graceful startup and shutdown. Later slices add capability packages and
adapters when observable behavior requires them. This ADR does not reserve
empty directories, placeholder interfaces, speculative services, or a complete
future tree.

The expected ownership pattern is:

- command packages compose the process and own no domain policy;
- capability packages own vocabulary, invariants, use cases, and the narrow
  ports they consume;
- provider or protocol adapters translate untrusted external representations;
- shared code exists only for semantics genuinely shared inside Argus, not as a
  `common`, `util`, or generic repository layer; and
- tests live beside the behavior they prove, with cross-capability and
  compatibility fixtures at the narrowest common boundary.

Go implementation not intended for external consumers SHOULD remain under
`internal`. An exported Go identifier or package is not a promise that another
repository may import it. Published reuse requires an explicit supported
contract and the extraction gate from ADR-0001.

### Contracts, data, adapters, and deployment assets

M02 language-neutral contracts will begin in this repository and will be
versioned independently from their filenames and generated language bindings.
Contract bundles MAY have their own artifact version while being built from the
same Git revision. A separate contracts repository is justified only when
external consumers require an independent release/support lifecycle that the
monorepo cannot provide safely.

Database migrations will live with the Argus code and schema behavior they
change. A future logical module MUST own its writes even if modules share one
database deployment. Splitting a service MUST NOT result in permanent
cross-service table writes; the extraction plan must establish an owned API,
event, or data migration boundary first.

SCM, CI, test-framework, model, storage, and Perfeng adapters begin in this
repository unless one of the split triggers below is demonstrated. A different
test family or vendor is not sufficient reason for another repository. Native
runner code MAY be extracted when it has an independent runtime, release,
distribution, or security lifecycle and communicates through a conformance-
tested adapter contract.

Local-development and deployment material begins here when an executable
runtime needs it. `perfeng-environment` does not become the owner of Argus
deployment by being attached to the same Codex project or local workspace.
Future shared environment composition requires the evidence and separate ADR
specified by ADR-0001.

### Perfeng and workspace relationship

Argus and every `perfeng-*` repository remain independent Git repositories.
Neither product is added as a Git submodule, subtree, vendored source copy, or
relative-path build dependency of the other. A developer workspace MAY attach
both product families for discovery and coordinated work, but workspace layout
MUST NOT become a build, runtime, or release contract.

Cross-product integration uses immutable artifact or contract versions and
documented compatibility. A coordinated change uses separate issues and pull
requests per repository, with the producer contract available before consumer
migration and deployment selection updated after compatible implementations
exist. No change relies on an atomic commit across repositories.

### Split triggers and non-triggers

A repository, module, process, or service split MAY be proposed when at least
one of these conditions is demonstrated and the separation improves the whole
lifecycle rather than only the local code layout:

1. **Independent consumers and releases:** multiple consumers need a supported
   artifact on a cadence or compatibility window different from Argus.
2. **Security or trust isolation:** credentials, data classification,
   authorization, or untrusted execution require an independently enforceable
   boundary.
3. **Runtime and failure isolation:** scaling, resource profile, availability,
   fault containment, or deployment location materially differs and has been
   observed or load-tested.
4. **Toolchain or distribution:** a native ecosystem, packaging format,
   platform matrix, or licensing obligation needs an independent deliverable.
5. **Ownership and operations:** a separately accountable owner can build,
   release, operate, support, and deprecate the component without coordinated
   repository changes.
6. **Proven cross-project reuse:** the candidate satisfies every common-library
   extraction criterion in ADR-0001.

The following are not sufficient by themselves:

- package, file, or line count;
- the existence of a logical capability boundary;
- a different test family or external provider;
- hypothetical future scale;
- the possibility that code could be reused;
- a desire for parallel development; or
- similarity to the current Perfeng polyrepo.

Every proposed split requires a new ADR. It MUST identify the trigger and
owner, public contract, state ownership, release and support policy, security
boundary, observability, local-development impact, rollout, rollback, and
deprecation of the old path. Where state or APIs already exist, the migration
MUST be incremental and compatibility-tested rather than a flag-day move.

## Alternatives considered

### Create a polyrepo matching Perfeng immediately

- Benefits: Clear repository names and independent CI/tooling from the start;
  superficial symmetry with an existing product family.
- Costs and risks: Every vertical slice would need coordinated repositories,
  versions, releases, local composition, and compatibility before independent
  ownership or operation exists.
- Reason not selected: Perfeng's topology reflects mature component lifecycles.
  Argus has not demonstrated those boundaries yet, and ADR-0001 already
  provides an extraction path when it does.

### Combine Argus and Perfeng in one repository

- Benefits: Atomic refactoring and one place for shared tooling and deployment.
- Costs and risks: It would conflate product authority, release cadence,
  credentials, state, contracts, and contributor workflows. It would also make
  apparent source proximity substitute for a supported integration contract.
- Reason not selected: The products cooperate but own different decisions and
  evidence. Their independent repositories enforce that boundary.

### Create one repository per test-family adapter

- Benefits: Framework-native tooling and independent adapter releases.
- Costs and risks: Most adapters initially need the same evolving identity,
  manifest, evidence, and conformance contracts. Early splits would amplify
  compatibility work and encourage inconsistent semantics.
- Reason not selected: Adapters begin in the Argus repository and split only
  when runtime, distribution, security, ownership, or consumer evidence meets a
  trigger.

### Begin with several services in one repository

- Benefits: Runtime boundaries are visible early while source remains together.
- Costs and risks: Network failure, authentication, deployment, compatibility,
  observability, and data ownership must be solved before the first useful
  workflow proves that independent operation is necessary.
- Reason not selected: A modular monolith preserves code boundaries without
  paying distributed-system costs prematurely.

## Consequences

### Positive

- Early vertical slices can change domain, adapters, contracts, tests, and
  documentation atomically within one review.
- One checkout and one primary command surface keep contributor setup and CI
  understandable during the foundation milestones.
- Internal package boundaries can evolve before becoming public compatibility
  commitments.
- Explicit split criteria make future extraction evidence-based and reviewable.
- Argus can inspect and integrate with Perfeng without inheriting its repository
  topology or implementation lifecycle.

### Negative

- A future extraction may require deliberate contract and state migration that
  an early polyrepo would already have forced.
- Multiple language toolchains may eventually coexist in one repository and
  increase the root verification surface.
- Repository-wide CI can become slower until checks are partitioned without
  weakening the required aggregate gate.
- Teams cannot release an internal component independently until its boundary
  passes the split criteria and migration review.

### Neutral or follow-up

- The next M01 task creates the root Go module and minimal control-plane command.
- Exact contract formats and bundle layout remain M02 work.
- PostgreSQL selection, schema, and migration tooling remain M03 work.
- Deployment packaging remains deferred until an executable runtime needs it.
- This ADR does not approve any separate Argus repository, service, or shared
  Argus–Perfeng component.

## Compatibility and migration

There is no runtime compatibility change because Argus has no executable
component. Existing documentation remains in place, and future code is added
incrementally to this repository.

An extracted component MUST retain a compatibility path for existing consumers
until they migrate to a released replacement. Contract producers publish the
new version before consumers adopt it. State-owning splits require an
expand/migrate/contract plan, verified data reconciliation, bounded dual-read or
dual-write only when unavoidable, explicit cutover authority, and rollback.
Repository history MAY be preserved during extraction, but history preservation
MUST NOT override least privilege or introduce a live source dependency.

## Security and operations

A single repository and deployable do not imply one ambient authority. Secrets
and external capabilities remain narrowly injected at composition boundaries.
Repository write access, CI dispatch, untrusted test execution, model access,
artifact access, and Perfeng requests MUST remain distinguishable and
least-privileged.

The modular monolith MUST preserve logical ownership and audit context so a
later security or operational split is possible. If untrusted generated or
repository-supplied code requires isolation, that workload executes in a
bounded worker environment even when its orchestrator source remains in this
repository.

## Validation

The decision is validated incrementally:

- the next scaffold MUST create one root Go module and one minimal production
  control-plane deployable without unused packages or services;
- repository checks MUST provide one aggregate command surface even if
  language-specific checks are added later;
- M02 contracts and their compatibility fixtures MUST be reproducible from this
  repository without importing private implementation packages;
- capability review MUST show inward dependencies and module-owned writes;
- workspace or sibling-repository paths MUST NOT appear as required build or
  runtime inputs;
- any proposed repository or service split MUST map its evidence to a listed
  trigger and include the required migration ADR; and
- cross-repository tests MUST use pinned published contracts or artifacts, not
  source-layout assumptions.
