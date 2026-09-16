# Contributing to Argus

## TL;DR

- Deliver one coherent, verified capability per issue and pull request.
- Name milestones MNN - Outcome, starting with M01.
- Use the required issue structure and state the milestone in every issue.
- Keep domain policy independent from frameworks and infrastructure.
- Follow the engineering and migration standards linked below.
- Include tests, documentation, and verification evidence with the change.
- At task completion, print the milestone and proposed GitHub issue.

## Sources of truth

| Concern | Source |
|:--|:--|
| Contributor and agent entry point | [AGENTS.md](AGENTS.md) |
| Engineering and language standards | [docs/development/engineering-standards.md](docs/development/engineering-standards.md) |
| PostgreSQL schema and migration rules | [docs/development/sql-migrations.md](docs/development/sql-migrations.md) |
| Architecture decisions | [docs/decisions](docs/decisions/README.md) |
| Milestones and planned work | [docs/roadmap/milestones.md](docs/roadmap/milestones.md) |
| Exact local and CI commands | Root README and checked-in tool configuration once introduced |

The principles in these documents are durable. Tool versions, generated
commands, environment variables, and service-specific setup belong beside the
implementation they operate.

## Before starting

- Read the product, architecture, and relevant ADRs when they exist.
- Confirm the target milestone and issue.
- Identify the smallest observable outcome that can be reviewed and merged
  independently.
- Identify affected contracts, migrations, security boundaries, documentation,
  and operational behavior.
- Stop and propose an ADR when the change affects compatibility, durability,
  security, data meaning, deployment topology, or multiple components.

## Milestones

Milestone titles use:

~~~text
MNN - Short outcome
~~~

Numbering starts at M01 and uses two digits through M99. Do not reuse or
renumber a published milestone. The text after the number describes the
capability available when the milestone is complete, not a team activity.

Every issue contains a Milestone line using the exact title. GitHub remains the
authoritative assignment after the milestone and issue have been created.

## Required issue structure

Use this title style:

~~~text
Imperative outcome in specific domain language
~~~

Prefer “Expose impacted tests for an API change” to “Implement impact service.”
Avoid prefixes that duplicate labels or milestones.

Every implementation task uses this body:

~~~markdown
**Milestone:** MNN - Outcome

## Goal

Describe the problem and observable result.

## Scope

- Included behavior and boundaries.

## Design decisions

- Important choices, constraints, compatibility effects, and ADR links.

## Acceptance criteria

- [ ] Observable behavior and verification evidence.
- [ ] Relevant failure or negative behavior.
- [ ] Documentation and operational effects.

## Out of scope

- Explicit exclusions and deferred work.
~~~

Acceptance criteria describe externally observable behavior or verifiable
invariants, not activities such as “create a class” or “write tests.” Record
known limitations rather than hiding them.

## Pull-request-sized work

A pull request should:

- solve one problem or deliver one coherent vertical capability;
- keep the repository buildable and deployable;
- include the implementation, tests, documentation, contract changes, and
  migration required for that capability;
- be reviewable without depending on an unmerged speculative follow-up; and
- contain evidence for both expected and important failure behavior.

Split work when it combines independent behavior, broad cleanup, dependency
upgrades, schema redesign, unrelated formatting, or several architectural
decisions. A change is not automatically too large because it touches several
layers; a thin vertical slice may legitimately include domain, application,
adapter, storage, and tests.

Do not create placeholder abstractions, empty packages, unused ports, or future
configuration merely to make a roadmap look implemented.

## Development workflow

1. Start from an issue assigned to an existing milestone.
2. Create a focused branch.
3. Add or update tests with the behavior.
4. Implement the smallest coherent solution.
5. Format and run the exact repository checks.
6. Review the complete diff for secrets, generated churn, accidental API
   changes, and unrelated edits.
7. Update documentation, ADRs, migration notes, and troubleshooting material.
8. Open a pull request linked to the issue and include verification evidence.

Use clear, imperative commit subjects. Keep generated output reproducible and
commit it only when consumers require it without running the generator.

## Documentation

Every long document contains a ## TL;DR section immediately after its title and
status metadata. A document is long when any of the following applies:

- it has 800 or more words;
- it has more than five second-level sections; or
- it is an architecture, end-to-end workflow, security, operational, or
  migration guide readers will consult selectively.

The TL;DR states the purpose, intended reader, important decision or outcome,
and next action where one exists, preferably in three to seven bullets. It does
not replace detailed safety constraints.

Documentation follows the same review standard as code. Examples should be
executable or verified where practical. Update or remove stale comments and
links in the same change that makes them inaccurate.

Architecture decisions use the template in
[docs/decisions/README.md](docs/decisions/README.md). Accepted ADRs are
historical records; supersede rather than rewrite them.

## Pull request description

A pull request states:

- linked issue and milestone;
- problem and resulting behavior;
- boundaries and deliberate exclusions;
- design and compatibility decisions;
- verification commands and evidence;
- migration, rollout, rollback, security, and operational considerations; and
- known limitations or follow-up work.

## Review checklist

- [ ] The issue belongs to a milestone and follows the required structure.
- [ ] The change is one coherent capability with explicit exclusions.
- [ ] Dependencies point inward and infrastructure does not leak into domain
      policy.
- [ ] Patterns and abstractions solve demonstrated needs rather than anticipated
      ones.
- [ ] Public contracts and persistence changes are compatible or have an
      approved evolution plan.
- [ ] Tests cover observable success and relevant negative/failure behavior.
- [ ] Concurrency, resource ownership, idempotency, and retries are explicit
      where applicable.
- [ ] Security, privacy, and untrusted-input boundaries were reviewed.
- [ ] Public APIs and non-obvious invariants are documented.
- [ ] Long documentation has a useful TL;DR.
- [ ] Migrations satisfy the migration criteria and upgrade verification.
- [ ] Formatting, type checking, static analysis, tests, race/concurrency
      checks, contract checks, and vulnerability checks pass as applicable.
- [ ] No secrets, production data, machine-specific paths, or unrelated changes
      are included.
- [ ] An ADR exists when the decision meets the ADR threshold.

## Completion report

After completing a work item, print the exact milestone, proposed GitHub issue
title, and complete issue body. Also report verification and limitations. This
keeps work performed before issue creation traceable to the same standard as
issue-first work.
