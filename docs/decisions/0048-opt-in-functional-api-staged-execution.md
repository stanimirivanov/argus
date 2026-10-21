# ADR-0048: Opt in to functional API staged execution

- Status: Proposed
- Date: 2026-10-21
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## Context

The shared staged invariants need an executable functional API consumer. Widening v1 stages would make old consumers interpret partial evidence as their existing selected/full-suite protocol. A schedule's own candidate list cannot prove that candidates or required policy were not omitted.

## Decision

Introduce explicit v2 bindings, plans, adapter requests and normalized attempts. Preserve strict v1 validation and default behavior. Both planner and runner verify the complete portfolio's raw bytes against trusted SHA-256, exact episode, independent complete manifest and reviewed revision/environment bindings. They use the shared partition validator before dispatch. The mode is independently required CI configuration, not a plan's self-authorization.

Shadow requires relevant-now plus an independent full control. Staged requires relevant-now plus its exact complement. Empty phases retain membership without adapter invocations. The runner executes one explicit phase; CI must run all obligatory later/control work even when early feedback fails. No release or promotion authority is added.

Preserve the portfolio early order; execute later/control tests in deterministic topological order. Reject cross-group dependencies because whole-group CLI jobs cannot enforce arbitrary interleaving. CI serializes jobs sharing resource capacity. These constraints are explicit failures rather than silently ignored scheduling declarations.

Keep result v1 because its exact normalized evidence semantics do not change. Version the request/attempt provenance instead. Extend the existing immutable attempt table with nullable metadata and a conditional version/stage/mode constraint. Leave v1 fingerprints byte-compatible by omitting absent metadata. Read v1/v2 evidence through an explicit dispatcher while keeping legacy-only consumers strict.

## Consequences and rollout

Deploy the forward migration and new planner/runner/evidence reader before opt-in; upgrade strict adapters to consume request v2. Existing writers remain valid. Old readers cannot interpret v2, so disable opt-in for application rollback while retaining the forward schema/evidence. Constraint validation scans and locks the attempt header table; no backfill or row rewrite is required. See the [operational guide](../integrations/functional-api-staged.md).

The portfolio remains an offline artifact; this slice runs an exact named episode without supplying live prediction or promotion. Review authentication, genuine CI evidence and checkout/environment provisioning remain external trust boundaries. Browser adoption, staged aggregate safety reporting and policy promotion remain M08-PR11/12/13.

## Verification

Run `make validate` and `make db-validate`. Tests cover exact stage requests, omissions/substitutions, mode mismatch, non-success evidence, cancellation, empty phases, v1 rejection, immutable provenance, concurrent retries, divergent reuse and released-schema upgrade.
