# ADR-0085: Validate frozen native policy claims

- Status: Proposed
- Date: 2026-10-30
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Pin native policy/v1 independently of Argus selection policy and review authority.
- Preserve exact numeric thresholds, baseline pins and omitted/configured sections.
- Reject ambiguous rules and contradictory bounds without computing outcomes.

## Decision and consequences

PR9d2b2a copies the policy schema and browser/checkout examples from contracts
commit 041273947703542f7fe22eab838c176df5517974, bundle 0.24.0, under the parent
snapshot Apache-2.0 license. Existing component pins remain unchanged. Reuse the
offline schema compiler, unique-key/depth/Unicode JSON and bounded numeric-token
controls, with a 1 MiB document limit and no new dependency.

Owned native policy claims preserve name/version/owner, observe/inform mode,
inconclusive missing-data behavior and the complete ordered rules. Each retains
metric selector, optional quality/SLO sections and regression direction/practical
difference/baseline ID/version. Numeric values use exact JSON number syntax;
omission differs from zero. Do not re-encode policy bodies to verify their digest.

In addition to the closed schema, reject duplicate rule IDs and name/statistic
selectors, even when units differ, matching native provider ambiguity guards.
Compare inclusive SLO min/max using bounded exact rationals so near-equal bounds
cannot reverse through rounding. These are definition checks, not statistical
computation or verdict approval. Unsupported generations and malformed input return
zero claims with safe errors; schema resources never load from the network.

This foundation supports both the future frozen-review loader and report/rule
conformance checks. The source of independently reviewed native bindings/exact
policy bytes still requires the explicit review-source decision; this decoder
establishes no authentication, approval or request association. Native/policy hash
binding, complete report rule/section verification, authenticated retrieval and
durable decision consumption remain separate. No encoder, I/O, clock, persistence,
migration, runtime activation, execution/blocking or overall PASS is added.
Revert the unused decoder without changing existing runtime or stored data.
