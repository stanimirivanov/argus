# ADR-0047: Define shared staged-execution invariants

- Status: Proposed
- Date: 2026-10-21
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Establish one runner-neutral partition/correlation boundary before functional API and browser adoption.
- Preserve complete candidate coverage and hard requirements in exact immutable scopes.
- Keep independent full controls mandatory in shadow mode and retain strict v1 validation.

## Context

The offline portfolio already exposes selected and omitted candidates. Its digest alone cannot prove that a runner received the correct repository revision, project, environment or complete inventory. Duplicating these invariants in two forthcoming runner protocols would risk inconsistent omissions and shadow-control semantics.

## Decision

Use the shared domain vocabulary and `scheduledplan` capability described in the [staged-execution guide](../integrations/staged-execution.md). A plan binds a verified portfolio episode and manifest to independent immutable group bindings and complete candidate inventories. Relevant-now preserves selected order and must contain every hard-required candidate. Remaining-later is the exact disjoint complement. Candidate decisions must be explicit: missing or unknown policy metadata is rejected instead of defaulting to optional.

Staged mode runs the two partition sets; shadow mode runs relevant-now followed by an independent full-candidate control. The remaining complement stays visible in shadow evidence but is covered by the control, avoiding a redundant third pass. Empty phases retain membership without requiring an invocation.

Consumers validate against independently supplied source and expected mode before obtaining stage tests. Domain code checks correlation and reference shape; artifact byte verification and source projection belong at the future transport boundary. New stage values remain invalid for existing v1 attempts/plans. PR10 and PR11 introduce the executable opt-in protocols.

## Consequences and compatibility

This is a foundation consumed by two later runner slices, not a new dispatch or persistence path. It adds no promotion authority. Full controls cannot be replaced by subsets through a plan's mode field. Group/candidate bounds, duplicate detection and owned copies make validation deterministic and bounded.

A schedule reference is declared provenance until its actual bytes and independent inventory are verified by the caller. Building an expected source from the untrusted plan defeats omission detection and is prohibited. Existing protocols retain their selected/full-suite behavior. Rollback before runner adoption is a normal revert; no migration is required.

## Verification

Run `make validate`. Focused execution tests cover partitions, mandatory tests, shadow control, invalid/cross-scope input, digest correlation, detached copies, empty phases and strict legacy attempt/API/browser plan validation. Architecture policy admits the new capability with dependencies pointing inward.
