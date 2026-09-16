# Argus product definition

**Status:** Canonical product scope
**Derived from:** Proposal reviewed 15 September 2026

## TL;DR

- Argus decides which tests should run and which tests may need maintenance for
  a software change.
- It combines deterministic Test Impact Analysis (TIA), evidence-calibrated
  Predictive Test Selection (PTS), prioritization, and constrained adaptation.
- Functional API and contract tests are the first vertical slice; UI follows
  after deterministic selection and validation are proven.
- Unit-test generation and maintenance are excluded from the initial product.
- Performance execution and analysis belong to Perfeng; Argus will later
  request and consume versioned performance evidence.
- Argus MUST prefer abstention or broader execution over an unsupported
  confident answer, and MUST never weaken a test oracle to obtain a green run.

## Purpose and audience

This document is the source of truth for what Argus is intended to achieve,
which test families it supports, and the safety envelope for automated test
maintenance. Product planning, architecture, issues, and acceptance criteria
MUST remain consistent with it. Durable architecture choices belong in ADRs;
delivery order belongs in the
[implementation milestones](../roadmap/milestones.md).

Argus serves test engineers, developers, repository owners, CI/platform teams,
and engineering leaders responsible for feedback speed and release confidence.

## Vision and outcomes

Argus maintains a continuously updated test portfolio that spends the available
testing budget where it provides the most useful evidence, stays synchronized
with the product, and gives people enough evidence to accept or reject every
automated decision.

The product has two coupled outcomes for each relevant change:

1. an **execution decision** describing which tests should run, when, and in
   what order; and
2. a **maintenance decision** describing which tests may be stale, what the
   smallest safe response is, and which review level is required.

Every material decision MUST be explainable and traceable to immutable source,
contract, policy, adapter, environment, and evidence revisions.

## Product principles

- **Safety before savings.** Missing or uncertain evidence expands execution or
  causes abstention; it does not silently narrow coverage.
- **Evidence before confidence.** A model score does not replace reproducible
  impact evidence, execution, invariant checks, or review.
- **Deterministic before generative.** Explicit mappings, semantic contracts,
  codemods, and constrained transforms are preferred over model-generated
  patches.
- **Oracles are protected.** Assertions, requirements, SLOs, security
  expectations, and approved baselines cannot be weakened to make a test pass.
- **Selection and maintenance stay connected.** An impacted broken test becomes
  a maintenance decision, not a convenient omission from the run.
- **Humans retain policy authority.** Repository owners define must-run rules,
  budgets, criticality, autonomy, and review requirements.
- **Autonomy is earned locally.** Promotion occurs per repository, test family,
  and adaptation class after measured shadow-mode evidence.
- **Customer-controlled operation.** Policies, adapters, execution, and
  evidence SHOULD be portable and deployable on customer-controlled systems.

## Capability model

Argus uses the following terms precisely:

| Capability | Question answered | Initial posture |
|:--|:--|:--|
| Test Impact Analysis (TIA) | Which tests may exercise behavior affected by this change? | Deterministic foundation |
| Regression Test Selection (RTS) | Which existing tests should be rerun? | Conservative subsets with fallback |
| Predictive Test Selection (PTS) | Which tests are most likely to provide useful failure evidence? | Shadow mode after historical baselines |
| Prioritization | In what order should selected tests run? | Explainable risk, value, cost, and constraints |
| Test adaptation | Is a test stale, and what is the smallest valid repair? | Constrained classes with validation |
| Test generation | Is important changed behavior uncovered? | Later, review-first capability |
| Test minimization | Are tests permanently redundant? | Advisory only; no early auto-deletion |

Selection, prioritization, and constrained adaptation form the initial product.
Generation and permanent minimization MUST NOT become prerequisites for the
first useful vertical slice.

## Test taxonomy and support policy

Test level, test type, and execution interface are separate dimensions. A test
may be system-level, functional, and API-facing at the same time. The support
policy below groups tests by the interface and evidence that most affect
selection and maintenance.

| Test family | Selection potential | Adaptation risk | Product posture |
|:--|:--|:--|:--|
| Functional API | High through routes, schemas, clients, traces, and service mappings | Low to high depending on oracle impact | First vertical slice in M05–M06 |
| API contract | High because the test basis is explicit | Low for compatible mechanics; high for semantic breaks | First vertical slice in M05–M06 |
| Component/service integration | Medium to high with runtime and dependency evidence | Medium because environments and data matter | Catalog now; adapters after API slice |
| Functional web UI | Medium through routes, components, accessibility structure, and history | Low for proven locator renames; high for flows/oracles | M07, Playwright first |
| End-to-end journeys | Medium; broad behavior lowers deterministic precision | High because business intent is often implicit | Selection after mappings mature; adaptation review-first |
| Accessibility | Medium to high for UI changes | Medium; expected violations MUST NOT be erased | M07 selection; conservative maintenance |
| Visual regression | Medium through route/component ownership | High because accepting a baseline can accept a defect | Selection may be supported; baseline changes require humans |
| Browser performance | Medium through UI journeys and capability mapping | High for budgets and baselines | Selected through Perfeng integration where applicable |
| Protocol performance | High when workloads map to APIs and capabilities | High for workloads, SLOs, and baselines | No direct adapter; integrate with Perfeng in M09 |
| Functional mobile/desktop UI | Medium; device and platform matrices dominate cost | Medium to high | Later, demand-driven adapter |
| Data/ETL/analytics | Medium through schema and lineage graphs | Medium when schemas are authoritative | Later, demand-driven adapter |
| Resilience/chaos/recovery | Medium through dependency topology | Very high and potentially destructive | Advisory only until isolated approval policy exists |
| Dynamic security testing | Medium through attack-surface changes | Very high; security oracles MUST NOT weaken | Outside initial scope; advisory selection later |
| Manual/exploratory/UAT | Recommendation rather than execution | No automatic rewrite | Traceability and recommendations only |
| Unit/component developer tests | High; mature TIA/PTS ecosystem | High-volume semantic coupling | Results may become signals; no initial generation or maintenance |
| Static/build verification | Usually cheap or already incremental | Configuration changes only | Must-run pipeline signals, not Argus-selected tests |

The initial focus on non-unit suites is deliberate: slow cross-repository tests
have high feedback and maintenance cost, while automatic mutation of fine-grain
developer tests creates large semantic risk. A later unit selection adapter MAY
be evaluated without making unit tests an architectural prerequisite.

## Execution decisions

For each change and candidate test, the decision engine produces one of these
outcomes with reasons and policy provenance:

| Outcome | Meaning |
|:--|:--|
| `RUN_REQUIRED` | Hard policy, direct impact, novelty, criticality, recent failure, or uncertainty requires execution. |
| `RUN_RECOMMENDED` | Evidence indicates high diagnostic value but no hard rule requires execution. |
| `DEFER` | Execute at a later stage such as post-merge, nightly, or pre-release. |
| `SKIP_FOR_NOW` | Omit from this stage with an explanation, expiry, and later control path. |
| `QUARANTINE` | Isolate a known flaky or infrastructure-failing test without hiding it. |
| `MANUAL_REVIEW` | Recommend a human exploratory, regulatory, or acceptance activity. |

Selection is constrained optimization, not one probability threshold. Critical
journeys, explicit pins, new or modified tests, security-sensitive changes, and
configured fallback suites MUST override a budget optimization. Tests omitted
from an early stage MUST have a defined remaining/full-suite execution policy.
Probabilistic subsets MUST NOT become release authority until owners explicitly
adopt a policy backed by local evidence.

## Maintenance decisions

For each impacted test, the maintenance engine produces one of these outcomes:

| Outcome | Meaning |
|:--|:--|
| `NO_CHANGE` | Available evidence says the test remains valid. |
| `INVALIDATED` | The test basis changed and requires attention. |
| `PATCH_AND_VALIDATE` | A constrained candidate may be changed and executed in isolation. |
| `PROPOSE_PR` | A candidate may be generated but requires human review. |
| `GENERATE_COVERAGE` | Important changed behavior appears to lack a suitable test. |
| `RETIRE_CANDIDATE` | Covered behavior appears removed; early releases never auto-delete it. |
| `ABSTAIN` | Evidence is missing, conflicting, unsupported, or below policy. |

A repaired test MUST NOT be trusted solely because it passes. The decision must
retain the original failure, candidate diff, validation results, and applicable
negative or discriminating evidence.

## Adaptation classes

### Class A — mechanically constrained

Argus MAY patch and execute a candidate in an isolated branch only when an
authoritative one-to-one mapping exists and configured invariants prove that
test intent is preserved. Candidate examples include:

- an endpoint, operation, or compatible field rename established by an
  authoritative contract and source change;
- a unique UI locator replacement supported by stable role, accessible name,
  test ID, or explicit source rename;
- a deterministic client, import, or framework syntax migration;
- replacement of a fixed delay with an observable state condition while
  preserving the asserted result; or
- a non-semantic fixture identifier refresh from an approved source.

“Automatic” means patch, execute, record, and route according to repository
policy. It never means an invisible runtime substitution. Auto-merge is a
separate policy that requires a proven observation period and instant rollback.

### Class B — model-assisted and review-first

Argus MAY prepare a pull request but MUST NOT merge automatically when a change
affects navigation or business flow, authentication or authorization, test
data lifecycle, assertions or expected values, snapshots, correlations not
described by a contract, exception/retry behavior, accessibility expectations,
visual baselines, performance workloads/SLOs/baselines, or multiple plausible
targets.

### Class C — human-led or prohibited

Argus MUST produce an issue, review recommendation, or abstention rather than a
patch for ambiguous requirements, unclear capability removal, weakened safety
or security expectations, destructive production operations, unapproved chaos
or high-load changes, and candidates supported only by “the changed test now
passes.”

## Validation gates

Every candidate MUST pass all applicable gates before it can advance:

1. repository parse, format, compile/type, lint, secret, policy, and dependency
   checks;
2. minimal diff limited to mapped tests and required helpers;
3. preservation of assertions and other protected oracles for Class A;
4. reproduction of the diagnosed original failure where practical;
5. repeated repaired success in a clean, controlled environment;
6. a negative, mutation, fault-seeding, contract, or equivalent discriminating
   check where the class requires it;
7. environment, data, workload, metric, and baseline compatibility;
8. flake evidence proportional to historical risk; and
9. assignment of required code and test owners.

If a required validator is unavailable, Argus MUST abstain. Diagnostic runtime
continuation MUST NOT silently turn a release gate green.

## Goals and non-goals

Product goals are to reduce feedback time and infrastructure cost without
silently reducing confidence; detect change impact across code, contracts,
configuration, data, dependencies, and repositories; identify stale or missing
coverage; produce minimal evidence-backed maintenance proposals; and learn from
explicit execution and review outcomes.

The first releases do not attempt to:

- replace test engineers, product owners, or repository policy;
- prove that a selected subset is equivalent to a full suite;
- silently change assertions, requirements, SLOs, security expectations, or
  approved baselines;
- generate or maintain unit tests;
- autonomously run destructive, chaos, penetration, or high-load tests;
- train a foundation model; or
- support every SCM, CI provider, framework, language, and test family.

## Success and promotion measures

Targets MUST be established from design-partner baselines, not copied from
vendor claims or research conducted in a different environment.

Selection evaluation includes failure recall, missed failures in remaining or
full runs, time to first useful failure, total cost and duration, selection rate
by family/criticality, and calibration error. Maintenance evaluation includes
validated-patch precision, oracle preservation, discriminating-check success,
reviewer edits, time to reviewed repair, reverts, and false healing. Portfolio
health includes critical capability coverage, unmapped tests, flake and
infrastructure trends, stale-test age, ownership gaps, and full-run frequency.

Evaluation MUST use chronological data, compare interpretable baselines, run
prospectively in shadow mode, publish misses and abstentions, include analysis
and review overhead, and report results by repository and test family. The
[implementation roadmap](../roadmap/milestones.md) owns promotion gates and
delivery order.
