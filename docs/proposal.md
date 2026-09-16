# Argus proposal decomposition record

**Status:** Historical proposal imported and decomposed
**Original research review:** 15 September 2026

## TL;DR

- The initial proposal expanded Argus from k6 maintenance into an adaptive test
  intelligence and evolution platform.
- Its durable content now lives in separate product, architecture, roadmap, and
  research sources of truth.
- This file records provenance and mapping; it is not a competing specification.
- Future changes MUST update the canonical destination rather than reconstruct
  the original monolithic proposal.

## Proposal outcome

The proposal established two connected product questions:

1. Which tests should run, when, and in what order for a software change?
2. Which tests have become stale, and what is the safest maintenance response?

It broadened scope from performance scripts to functional API, contract,
integration, browser UI, end-to-end, accessibility, visual, data, performance,
and advisory test families. It also introduced deterministic Test Impact
Analysis, Predictive Test Selection, budget-aware prioritization, constrained
adaptation classes, evidence-based validation, and learning from review and
delayed outcomes.

The proposal also corrected three early assumptions:

- decisions should begin during pull requests rather than only after merge;
- “self-healing” must not hide product failures or weaken test oracles; and
- Argus should integrate with Perfeng instead of owning k6 execution,
  performance statistics, SLO evaluation, baselines, or raw evidence.

## Canonical destination map

| Original proposal content | Canonical destination |
|:--|:--|
| Vision, goals, non-goals, terminology, taxonomy, decision outcomes, adaptation classes, validation, and success measures | [Product definition](product/product-definition.md) |
| System context, logical capabilities, information flow, evidence invariants, adapters, trust boundaries, and Perfeng responsibilities | [Architecture overview](architecture/overview.md) |
| Delivery phases, immediate work, sequencing, and promotion gates | [Implementation milestones](roadmap/milestones.md) |
| Research findings, open-source references, commercial offerings, competitive thesis, and open questions | [Research and market landscape](research/landscape.md) |
| Durable technology and repository choices | [Architecture decision records](decisions/README.md), once proposed and accepted |

When two sources appear to disagree, use the authority rules in
[CONTRIBUTING.md](../CONTRIBUTING.md). This decomposition record provides
history only and MUST NOT override a canonical destination or accepted ADR.

## Decisions retained from the proposal

- Functional API and contract tests form the first end-to-end vertical slice.
- UI selection and constrained locator repair follow after the API evidence and
  validation model is proven.
- Predictive selection begins with interpretable baselines and shadow-mode
  evaluation.
- Unit-test generation and maintenance are excluded from the initial releases.
- Permanent test deletion is advisory only during early product stages.
- Model-assisted changes remain bounded by deterministic policy and independent
  validation.
- Performance integration is deferred until Argus identity and catalog
  contracts are stable.
- Autonomy is promoted independently per repository, test family, and
  adaptation class.

## Material proposal content deliberately deferred

The proposal named candidate technologies and repository splits. Those are not
accepted merely because they appeared in a proposal. Control-plane language,
component boundaries, repository topology, storage, eventing, contract formats,
model provider, and deployment packaging require the ADR or milestone listed in
the [architecture overview](architecture/overview.md#deliberately-deferred-decisions).

Calendar estimates from the proposal were not made canonical. Argus uses
outcome-based milestones and evidence-based promotion gates because delivery
rate depends on design-partner repositories, available history, CI integration,
and measured safety.
