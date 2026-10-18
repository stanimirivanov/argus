# ADR-0035: Plan browser controls from reviewed project bindings

- Status: Proposed
- Date: 2026-10-18
- Milestone: M07 - UI test intelligence
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Reviewed CI configuration binds immutable browser test revisions, projects,
  environments, and stable group keys; selection evidence cannot provide
  executable commands or credentials.
- Every nonempty group has a full-suite control; a selected job exists only
  when the manifest requires an early test for that project.
- Compare only explicit immutable browser attempt IDs for all planned groups.
  Missing controls and cross-project results invalidate the report.
- The full-suite outcome remains the CI and release authority. Shadow reports
  measure duration and failure recall, including misses in full-only groups.

## Context

The browser selection manifest carries stable tests and project hints, while
the browser runner executes exactly one repository/project/environment group.
Without a reviewed planner, CI could omit a control or interpret an empty
project hint as permission to drop tests. Comparing whichever attempt was
stored last could mix reruns, revisions, or environments.

## Decision

Introduce browser-specific v1 execution bindings, a flat execution plan,
explicit plan-attempt bindings, and a complete shadow report. The planner
checks immutable repository coordinates and test revisions against every
manifest decision. A project-less fallback applies to all reviewed projects
for that repository. Targeted project hints must each be covered. Duplicate
and empty groups fail. Jobs contain no command, credential, or artifact path.

The comparator requires one full-suite attempt for every group and a selected
attempt exactly when planned. It checks the manifest digest, revision, stage,
repository, adapter, project, environment, result count, and adapter version.
It never infers attempt identity from recency. A missing or invalid attempt
prevents a partial report. Failed and incomplete full-suite attempts remain
authoritative for release gating; reporting success cannot override them.

The browser manifest SHA-256 covers the exact JSON bytes shared with
`run-browser`. The plan SHA-256 covers canonical plan JSON. Reformatting the
manifest between planning and execution changes its identity and fails
correlation.

## Alternatives considered

### Reuse functional API plan contracts

Those contracts omit browser project and environment dimensions. Reinterpreting
their v1 fields would make stored API evidence ambiguous.

### Pick the newest attempt per group

This could pair different revisions or reruns. Explicit IDs are required.

### Skip full-only groups

This would hide every miss from a group with no early-stage tests. Controls
are mandatory for every group.

## Consequences

CI must maintain reviewed group bindings and preserve each manifest file
unchanged. It must ingest valid failed attempts and retain artifact bytes
separately from stored references. The reference campaign demonstrates the
workflow; production CI keeps its own command, credential, isolation, and
release policy.
