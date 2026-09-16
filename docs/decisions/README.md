# Architecture decision records

## TL;DR

- Use an ADR for durable decisions affecting compatibility, security, data,
  topology, ownership, or multiple components.
- Number ADRs sequentially and never reuse a number.
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

No ADRs have been accepted yet.
