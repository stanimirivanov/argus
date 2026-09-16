# ADR-0001: Use Go and evidence-based cross-project reuse

- Status: Accepted
- Date: 2026-09-16
- Milestone: M01 - Engineering foundation
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Implement the Argus operational control plane in Go as a modular monolith.
- Keep domain and application policy independent from transports, persistence,
  orchestration platforms, model providers, and Perfeng.
- Introduce Python only behind a versioned process or artifact boundary when a
  concrete analysis or evaluation workload justifies it.
- Do not import, fork, or copy Perfeng control-plane internals. Reuse published
  contracts, conformance fixtures, and proven design approaches first.
- Extract a common library only after two real consumers demonstrate identical
  semantics, lifecycle, security, and compatibility needs.
- Keep Argus and Perfeng independently deployable, with separate state and
  authority. Shared deployment composition requires a later decision.

## Context

Argus needs a long-running control plane for authenticated change ingestion,
catalog and impact decisions, manifest publication, result ingestion,
maintenance orchestration, and auditable background work. Its safety properties
require explicit state, bounded concurrency, cancellation, idempotency, typed
failure classification, reproducible builds, and controlled shutdown. Later
milestones add PostgreSQL persistence, CI and SCM adapters, and potentially
Kubernetes-hosted execution or validation workers.

The attached Perfeng repositories provide relevant implementation evidence:

- `perfeng-control-plane` uses Go for a durable HTTP and reconciliation runtime
  with explicit domain state, leases, revision checks, PostgreSQL, Kubernetes,
  object storage, and graceful process lifecycle.
- Its reusable-looking code is intentionally under Go `internal` packages and
  implements performance-specific run, baseline, artifact, registry, and
  reconciliation semantics. It is not a supported general-purpose library.
- `perfeng-contracts` owns language-neutral schemas, OpenAPI descriptions,
  examples, compatibility rules, and conformance fixtures. Its Python code is
  repository validation tooling rather than a consumer runtime dependency.
- `perfeng-environment` owns Perfeng workspace and deployment composition. It
  does not own component behavior or a general multi-product platform.

Using the same language can reduce duplicated operational learning, but shared
language alone does not establish shared domain meaning. Directly reusing
Perfeng implementation would couple Argus to performance-specific lifecycle,
release, security, storage, and deployment decisions. Conversely, refusing all
reuse would discard proven patterns and make future genuinely common behavior
harder to recognize.

This decision therefore selects both the control-plane language and the rule by
which cross-project reuse may be introduced.

## Decision

### Control-plane language and component boundary

The Argus operational control plane will be implemented in Go. The initial
runtime will be a modular monolith: one deployable process may contain multiple
capability-oriented packages, but a capability does not become a network
service merely because it has a distinct domain boundary.

Go owns the initial production paths for:

- process lifecycle and configuration;
- change ingestion and normalization orchestration;
- repository and test catalog application behavior;
- deterministic impact, selection, and maintenance policy;
- execution-manifest and evidence coordination;
- result and review-outcome ingestion; and
- durable work coordination when a milestone demonstrates that need.

Domain packages MUST NOT import HTTP, SQL, Kubernetes, SCM, CI, object-store,
model-provider, or Perfeng types. Application packages define the ports they
consume, and adapters translate external representations into validated Argus
values. The first scaffold MUST establish this direction without pre-creating
unused layers or interfaces.

Python remains an allowed implementation language for a separately testable
analysis or evaluation component when an actual workload benefits from its
scientific or machine-learning ecosystem. Expected examples are historical
replay, statistical calibration, feature preparation, and model evaluation in
M08. Such a component MUST exchange versioned, language-neutral inputs and
outputs through a process, job, or immutable-artifact boundary. It MUST NOT
share in-memory objects or rely on private Go representations. No Python
service is created by this decision.

TypeScript MAY be used where a browser or framework adapter requires its native
ecosystem. It is not a second implementation language for control-plane domain
policy.

### Perfeng integration and reuse boundary

Argus and Perfeng remain independently versioned products and deployments. Each
owns its domain state, credentials, authorization decisions, operational
lifecycle, and database schema. Neither product MAY read or write the other's
private tables, import the other's internal domain types, or coordinate by
assuming filesystem layout or deployment internals.

Argus will not add a Go module dependency on `perfeng-control-plane`, fork its
packages, or copy substantial implementations from it. Argus MAY adopt proven
design approaches—such as validated startup composition, consumer-owned ports,
explicit lifecycle state, lease fencing, immutable evidence references, and
secret-safe error classification—using Argus-specific requirements and tests.

Performance exchange will occur through published, language-neutral contracts.
Perfeng remains authoritative for performance workload, environment, baseline,
measurement-quality, analysis, and raw-artifact meaning. Argus remains
authoritative for cross-test-family change impact and the decision to request
performance evidence. M09 will define and jointly review the precise request,
acceptance or rejection, completion, and invalidation contracts. Until a stable
release exists, a consumer MUST pin the exact accepted contract-bundle revision
rather than a floating branch.

Perfeng-specific contracts remain owned by `perfeng-contracts`. General Argus
test-decision contracts remain owned by Argus. A neutral contract repository is
not created pre-emptively; it MAY be proposed later if a genuinely shared
contract needs independent ownership and release cadence.

### Common-library extraction gate

Duplication is preferable to the wrong shared abstraction while behavior is
still being discovered. A common library MAY be proposed only when all of the
following are demonstrated:

1. At least two production consumers need the same behavior now.
2. The behavior has identical domain meaning, validation, failure,
   cancellation, security, and compatibility semantics for those consumers.
3. Its public API can use domain-neutral language without leaking either
   product's state machine, persistence model, or vendor choices.
4. Conformance tests or fixtures can specify observable behavior independently
   from either implementation.
5. An owner, versioning policy, support window, release process, and consumer
   migration path are explicit.
6. Consumers can upgrade independently and have a rollback or replacement
   path.
7. Extraction has lower lifecycle cost than keeping two small implementations.

The preferred reuse order is:

1. vocabulary and documented semantics;
2. schemas, examples, and conformance fixtures;
3. small library with a narrow stable API; and only then
4. a shared service when independent operation and ownership justify one.

Potential future candidates include content identity, immutable provenance,
or contract-conformance primitives. They are not approved by this ADR. Perfeng
run lifecycle, baseline policy, reconciliation, Kubernetes jobs, object-store
composition, authentication, and database repositories are explicitly not
common-library candidates at this stage because their meaning and operational
authority are product-specific.

### Environment composition

Argus does not depend on `perfeng-environment` for development or deployment.
That repository MAY later compose an Argus release only through an explicit,
pinned deployment contract that preserves independent namespaces, identities,
state, credentials, upgrades, and rollback. Extracting a neutral environment or
GitOps composition project requires demonstrated needs from both products and a
separate topology ADR; the current Perfeng Argo CD refactoring does not decide
the Argus deployment model.

Cross-project changes MUST be delivered as coordinated but independently
reviewable repository changes. The producing contract change is versioned
first, consumers migrate under an explicit compatibility window, and deployment
selection changes last. Every repository runs its own checks and retains its
own issue, pull request, and release history.

## Alternatives considered

### Use Python for the complete control plane

- Benefits: One language for service logic and future data-science work; rapid
  experimentation; broad analysis ecosystem.
- Costs and risks: It would not reuse the proven Go operational experience from
  Perfeng, and it would combine asynchronous service lifecycle concerns with
  analysis concerns that have different dependency and release profiles.
- Reason not selected: Argus's initial work is a durable operational control
  plane, while Python's strongest anticipated need is a later bounded analysis
  workload. A versioned boundary preserves that option without selecting it
  prematurely.

### Use TypeScript for the complete control plane

- Benefits: Strong browser and web tooling alignment; one language for future
  Playwright-facing adapters.
- Costs and risks: Browser adapters are only one boundary, and choosing their
  ecosystem for the core would provide no demonstrated advantage for durable
  orchestration, catalog, and policy behavior.
- Reason not selected: TypeScript remains appropriate at native adapter edges,
  but it is not the best evidence-backed default for the control plane.

### Generalize or import `perfeng-control-plane`

- Benefits: Superficially faster access to working HTTP, PostgreSQL,
  Kubernetes, artifact, and reconciliation code.
- Costs and risks: The code intentionally uses internal packages and encodes
  Perfeng run, baseline, registry, artifact, and lifecycle semantics. Making it
  generic now would destabilize Perfeng and couple both products' releases.
- Reason not selected: Similar infrastructure mechanisms do not establish
  identical domain behavior. Contract interoperability and selective future
  extraction preserve independence.

### Create a shared foundation repository immediately

- Benefits: A visible home for common code and standards from the start.
- Costs and risks: It would create ownership, compatibility, release, and
  dependency obligations before a second concrete implementation proves the
  abstraction.
- Reason not selected: The extraction gate supplies a path to create one when
  evidence exists. Creating it now would be speculative architecture.

### Split Argus into Go and Python services immediately

- Benefits: Early runtime isolation and independent scaling.
- Costs and risks: Network contracts, deployment, observability, failure
  handling, and version coordination would be added before a Python-owned use
  case exists.
- Reason not selected: Start as a modular monolith and extract only around a
  measured workload or independent operational boundary.

## Consequences

### Positive

- The next executable task has an unambiguous language and process boundary.
- Argus can reuse the team's Go operating knowledge without inheriting Perfeng
  domain coupling.
- Language-neutral contracts support Go, Python, and TypeScript consumers and
  make compatibility observable.
- The extraction gate limits implementation drift while preventing a premature
  common-platform project.
- Product authority, state ownership, credentials, and release cadence remain
  explicit.

### Negative

- Some small infrastructure mechanisms may initially be implemented twice.
- Later analysis may introduce a polyglot build and an additional deployment or
  job boundary.
- Coordinated contract evolution requires compatibility fixtures and ordered
  pull requests rather than direct type sharing.
- A useful common library cannot be extracted merely because code looks
  similar; semantic and operational equivalence MUST be demonstrated.

### Neutral or follow-up

- The next M01 task scaffolds the Go module and minimal signal-aware command.
- Repository topology remains a separate M01 ADR.
- Contract representation and generated bindings remain M02 decisions.
- Durable work ownership, PostgreSQL, and deployment topology remain deferred
  until their roadmap slices require them.
- M09 will make the Argus–Perfeng wire boundary concrete.

## Compatibility and migration

This decision introduces no runtime or wire compatibility change because Argus
has no executable component yet. The first Go scaffold establishes the module
and supported toolchain. Future Python or shared-library introductions require
their own compatibility and rollout plan if they cross process, repository, or
published API boundaries.

If a future ADR replaces Go for the control plane, it MUST define coexistence,
state and API migration, operational rollback, and removal of the old runtime.
If common code is extracted from an existing product, the source repository
MUST first retain a compatibility adapter so extraction is not a flag-day
change.

## Security and operations

Language choice does not relax Argus security boundaries. Repository read and
write authority, CI dispatch, artifact access, model access, and Perfeng
requests remain separate least-privilege capabilities. Argus and Perfeng use
separate service identities and private persistence unless a future ADR proves
that a shared operational boundary is safe.

Cross-process and cross-product input is untrusted and MUST be authenticated,
bounded, schema-validated, correlated, and handled idempotently. Shared
contracts do not grant authorization, and a shared library MUST NOT create
ambient credentials or global mutable state.

## Validation

The decision is validated incrementally:

- the next M01 scaffold MUST build and stop gracefully using the selected Go
  toolchain;
- architecture tests and package review MUST show dependencies pointing from
  adapters toward application and domain code;
- no Argus module MAY import `perfeng-control-plane` internal implementation;
- M02 compatibility tests MUST consume language-neutral fixtures without
  relying on one implementation language;
- any common-library proposal MUST supply evidence for every extraction-gate
  criterion; and
- M09 integration tests MUST demonstrate independently versioned Argus and
  Perfeng components exchanging pinned contracts without private-state access.
