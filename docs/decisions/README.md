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
| [ADR-0003](0003-use-json-schema-and-generated-contract-bindings.md) | Use JSON Schema and generated contract bindings | Accepted | 2026-09-16 |
