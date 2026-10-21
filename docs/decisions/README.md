# Architecture decision records

## TL;DR

- Use an ADR for durable decisions affecting compatibility, security, data,
  topology, ownership, or multiple components.
- Number ADRs sequentially and never reuse a number.
- Allocate a number from fresh repository state; renumber after a concurrent
  collision instead of forcing a merge.
- Accepted ADRs are historical records; supersede rather than rewrite them.
- Record alternatives, consequences, rollout, and validation—not only the
  chosen technology.

## When an ADR is required

Create an ADR when a decision materially affects one or more of:

- public contracts or compatibility;
- persistence, durability, data ownership, or migration strategy;
- security, privacy, authorization, or trust boundaries;
- service/repository/deployment topology;
- language, framework, database, queue, model provider, or build foundation;
- cross-component operational behavior; or
- a constraint that future contributors might otherwise “simplify” away.

Local implementation choices that are cheap to reverse do not need an ADR.

## Naming and lifecycle

Use a four-digit sequence and a short kebab-case name:

~~~text
0001-use-go-for-the-control-plane.md
~~~

Statuses are Proposed, Accepted, Rejected, Deprecated, or Superseded. A
superseded ADR links to its replacement. Do not edit the decision or
consequences of an accepted ADR to make history appear cleaner; add a note or a
new ADR.

Add every ADR to the index below.

Immediately before creating an ADR, refresh the branch as the current workflow
allows and inspect both this index and the decision directory. Allocate the
lowest unused four-digit number from that state. The filename does not reserve
the number outside the proposed change.

If concurrent work uses the same number, rebase or otherwise refresh the branch,
rename the later ADR to the next available number, and update its title, index
entry, and references. Treat this as a renumbering operation, not a content
merge to force through. Accepted or published ADR numbers remain immutable.

## Template

~~~markdown
# ADR-NNNN: Decision title

- Status: Proposed
- Date: YYYY-MM-DD
- Milestone: MNN - Outcome
- Deciders:
- Supersedes:
- Superseded by:

## Context

What forces a decision now? Include constraints and quality attributes.

## Decision

State the decision and its scope precisely.

## Alternatives considered

### Alternative

- Benefits:
- Costs and risks:
- Reason not selected:

## Consequences

### Positive

### Negative

### Neutral or follow-up

## Compatibility and migration

## Security and operations

## Validation

How will the assumptions and consequences be verified?
~~~

## Index

| ADR | Decision | Status | Date |
|:--|:--|:--|:--|
| [ADR-0001](0001-use-go-and-evidence-based-cross-project-reuse.md) | Use Go and evidence-based cross-project reuse | Accepted | 2026-09-16 |
| [ADR-0002](0002-keep-argus-in-a-single-product-repository.md) | Keep Argus in a single product repository | Accepted | 2026-09-16 |
| [ADR-0003](0003-use-effect-schema-at-contract-boundaries.md) | Use Effect Schema at contract boundaries | Accepted | 2026-09-16 |
| [ADR-0004](0004-use-postgresql-and-embedded-forward-migrations.md) | Use PostgreSQL and embedded forward migrations | Accepted | 2026-09-17 |
| [ADR-0005](0005-store-immutable-impact-evidence.md) | Store immutable impact evidence and derive edge state at query time | Accepted | 2026-09-18 |
| [ADR-0006](0006-enforce-capability-oriented-hexagonal-boundaries.md) | Enforce capability-oriented hexagonal boundaries | Accepted | 2026-09-18 |
| [ADR-0007](0007-ingest-github-changes-as-bounded-immutable-evidence.md) | Ingest GitHub changes as bounded immutable evidence | Accepted | 2026-09-18 |
| [ADR-0008](0008-derive-capability-impact-from-openapi-operations.md) | Derive capability impact from OpenAPI operations | Accepted | 2026-09-18 |
| [ADR-0009](0009-start-functional-api-selection-with-deterministic-safe-fallbacks.md) | Start functional API selection with deterministic safe fallbacks | Accepted | 2026-09-18 |
| [ADR-0010](0010-use-a-bounded-process-protocol-for-functional-api-execution.md) | Use a bounded process protocol for functional API execution | Accepted | 2026-09-19 |
| [ADR-0011](0011-store-immutable-attempts-and-compare-explicit-shadow-pairs.md) | Store immutable attempts and compare explicit shadow pairs | Accepted | 2026-09-26 |
| [ADR-0012](0012-plan-execution-from-reviewed-repository-bindings.md) | Plan execution from reviewed repository bindings | Accepted | 2026-09-26 |
| [ADR-0013](0013-evaluate-selection-across-complete-execution-plans.md) | Evaluate selection across complete execution plans | Accepted | 2026-09-26 |
| [ADR-0014](0014-generate-endpoint-repair-proposals-through-reviewed-adapters.md) | Generate endpoint repair proposals through reviewed adapters | Accepted | 2026-09-26 |
| [ADR-0015](0015-validate-repairs-with-a-restored-negative-control-workspace.md) | Validate repairs with a restored negative-control workspace | Accepted | 2026-09-27 |
| [ADR-0016](0016-publish-validated-repairs-as-idempotent-draft-pull-requests.md) | Publish validated repairs as idempotent draft pull requests | Accepted | 2026-09-27 |
| [ADR-0017](0017-capture-terminal-review-outcomes-as-bounded-evidence.md) | Capture terminal review outcomes as bounded evidence | Accepted | 2026-09-27 |
| [ADR-0018](0018-store-one-immutable-outcome-per-adaptation-review.md) | Store one immutable outcome per adaptation review | Accepted | 2026-09-27 |
| [ADR-0019](0019-preserve-trustworthy-validation-rejections.md) | Preserve trustworthy validation rejections | Accepted | 2026-09-28 |
| [ADR-0020](0020-scope-postgresql-runtime-stores-by-capability.md) | Scope PostgreSQL runtime stores by capability | Proposed | 2026-10-01 |
| [ADR-0021](0021-keep-go-contract-dtos-product-internal.md) | Keep Go contract DTOs product-internal | Proposed | 2026-10-01 |
| [ADR-0022](0022-keep-purpose-specific-command-boundaries.md) | Keep purpose-specific command boundaries | Proposed | 2026-10-01 |
| [ADR-0023](0023-separate-github-review-read-and-write-authority.md) | Separate GitHub review read and write authority | Proposed | 2026-10-02 |
| [ADR-0024](0024-own-endpoint-repair-proposals-by-test-family.md) | Own endpoint repair proposals by test family | Proposed | 2026-10-02 |
| [ADR-0025](0025-select-browser-tests-only-for-fully-covered-api-changes.md) | Select browser tests only for fully covered API changes | Proposed | 2026-10-02 |
| [ADR-0026](0026-check-playwright-inventory-against-declared-tests.md) | Check Playwright inventory against declared tests | Proposed | 2026-10-03 |
| [ADR-0027](0027-opt-in-browser-selection-from-declared-component-roots.md) | Opt in to browser selection from declared component roots | Proposed | 2026-10-03 |
| [ADR-0028](0028-evaluate-embedded-dbos-workflows-for-change-processing.md) | Evaluate embedded DBOS workflows for change processing | Proposed | 2026-10-04 |
| [ADR-0029](0029-migrate-stable-contract-toolchain-without-wire-changes.md) | Migrate the stable contract toolchain without wire changes | Proposed | 2026-10-13 |
| [ADR-0030](0030-bind-reviewed-functional-api-policy-to-complete-head-inventory.md) | Bind reviewed functional API policy to complete head inventory | Proposed | 2026-10-15 |
| [ADR-0031](0031-execute-reviewed-functional-api-manifests-through-the-existing-protocol.md) | Execute reviewed functional API manifests through the existing protocol | Proposed | 2026-10-16 |
| [ADR-0032](0032-propose-compatible-field-renames-from-explicit-contract-identity.md) | Propose compatible field renames from explicit contract identity | Proposed | 2026-10-17 |
| [ADR-0033](0033-validate-field-repairs-with-repeated-discriminating-runs.md) | Validate field repairs with repeated discriminating runs | Proposed | 2026-10-17 |
| [ADR-0034](0034-persist-reviewed-browser-discovery-before-targeted-selection.md) | Persist reviewed browser discovery before targeted selection | Proposed | 2026-10-18 |
| [ADR-0035](0035-plan-browser-controls-from-reviewed-project-bindings.md) | Plan browser controls from reviewed project bindings | Proposed | 2026-10-18 |
| [ADR-0036](0036-propose-browser-locator-edits-from-correlated-structure.md) | Propose browser locator edits from correlated structure | Proposed | 2026-10-18 |
| [ADR-0037](0037-validate-browser-locator-repairs-with-artifact-bound-runs.md) | Validate browser locator repairs with artifact-bound runs | Proposed | 2026-10-19 |
| [ADR-0038](0038-publish-browser-locator-repairs-for-human-review.md) | Publish validated browser locator repairs for human review | Proposed | 2026-10-19 |
| [ADR-0039](0039-capture-explicit-predictive-observations.md) | Capture explicit predictive observations for offline evaluation | Proposed | 2026-10-20 |
| [ADR-0040](0040-replay-predictive-evidence-with-a-fixed-temporal-cutoff.md) | Replay predictive evidence with a fixed temporal cutoff | Proposed | 2026-10-20 |
| [ADR-0041](0041-compare-predictive-baselines-with-training-only-signals.md) | Compare predictive baselines with training-only signals | Proposed | 2026-10-20 |
| [ADR-0042](0042-train-a-causal-interpretable-ranking-baseline.md) | Train a causal interpretable ranking baseline | Proposed | 2026-10-20 |
| [ADR-0043](0043-calibrate-predictive-risk-by-repository-and-family.md) | Calibrate predictive risk by repository and family | Proposed | 2026-10-21 |
| [ADR-0044](0044-abstain-on-weak-predictive-evidence.md) | Abstain on weak predictive evidence | Proposed | 2026-10-21 |
| [ADR-0045](0045-estimate-stage-cost-from-reviewed-constraints.md) | Estimate stage cost from reviewed constraints | Proposed | 2026-10-21 |
| [ADR-0046](0046-schedule-diverse-budgeted-portfolios.md) | Schedule diverse budgeted portfolios | Proposed | 2026-10-21 |
| [ADR-0047](0047-define-shared-staged-execution-invariants.md) | Define shared staged-execution invariants | Proposed | 2026-10-21 |
