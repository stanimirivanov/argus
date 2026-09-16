# Contributing to Argus

## TL;DR

- Deliver one coherent, verified capability per issue and pull request.
- Use MUST, SHOULD, and MAY as defined below; ordinary lowercase wording is
  explanatory rather than a separate hidden policy.
- Proceed on documented, reversible assumptions; stop for decisions that
  materially affect behavior, safety, compatibility, scope, or external state.
- Use an existing issue when one is supplied. Otherwise complete the smallest
  coherent scope and propose the full issue in the completion report.
- Run the exact repository checks. If a check cannot run, report it as not run
  with the reason and residual risk—never as passed.
- Name milestones `MNN - Outcome`, starting with M01, and state the milestone
  in every issue.
- Keep domain policy independent from frameworks and infrastructure.
- Include tests, documentation, assumptions, and verification evidence with
  the change.

## Policy language and sources of truth

The uppercase terms in this repository have these meanings:

- **MUST** and **MUST NOT** identify requirements. A deviation needs an explicit
  reviewer-approved exception recorded in the issue or pull request.
- **SHOULD** and **SHOULD NOT** identify strong defaults. A deviation is allowed
  only when its reason and trade-off are recorded.
- **MAY** identifies an optional choice.

Lowercase words such as “prefer,” “avoid,” and “may” provide guidance and do
not silently create another level of normative strength.

| Concern | Canonical source |
|:--|:--|
| Contributor workflow, issue timing, escalation, and completion | This document |
| Concise contributor and agent entry point | [AGENTS.md](AGENTS.md) |
| Product scope, taxonomy, decision outcomes, and safety | [docs/product/product-definition.md](docs/product/product-definition.md) |
| Conceptual architecture and system boundaries | [docs/architecture/overview.md](docs/architecture/overview.md) |
| Engineering and language standards | [docs/development/engineering-standards.md](docs/development/engineering-standards.md) |
| Supported local environments and first-time setup | [docs/development/developer-quickstart.md](docs/development/developer-quickstart.md) |
| PostgreSQL schema and migration rules | [docs/development/sql-migrations.md](docs/development/sql-migrations.md) |
| Architecture decisions | [docs/decisions](docs/decisions/README.md) |
| Milestones and planned work | [docs/roadmap/milestones.md](docs/roadmap/milestones.md) |
| Research and market evidence | [docs/research/landscape.md](docs/research/landscape.md) |
| Exact local and CI commands | Root README and checked-in tool configuration |

When sources conflict, the narrower source governs its stated concern. An
accepted ADR governs the decision it records until it is superseded. A
contributor MUST surface an unresolved conflict rather than quietly choosing
the convenient rule.

The principles in these documents are durable. Tool versions, generated
commands, environment variables, and service-specific setup belong beside the
implementation they operate.

## Before starting

A contributor MUST:

- inspect the working tree and preserve changes that are not part of the task;
- read the product, architecture, relevant standards, and accepted ADRs;
- identify the smallest observable outcome that can be reviewed and merged
  independently;
- identify affected contracts, migrations, security boundaries, documentation,
  and operational behavior; and
- determine the applicable issue workflow under [Issue timing](#issue-timing).

A contributor MUST NOT discard, overwrite, reformat, or incorporate unrelated
work merely to obtain a clean diff. Destructive operations and externally
visible writes outside the stated task require explicit authorization.

An ADR MUST be proposed when a change establishes or revises a durable decision
about compatibility, persistence or data meaning, security, deployment
topology, foundational technology, ownership, or cross-component behavior.
Ordinary implementation ambiguity does not require an ADR.

## Ambiguity and escalation

Before escalating, a contributor SHOULD inspect relevant code, tests, fixtures,
documentation, ADRs, and issue history and perform safe read-only investigation.

A contributor MAY proceed with a documented assumption when all of the
following are true:

- the choice stays within the stated outcome and exclusions;
- it is local, reversible, and inexpensive to change;
- it does not alter a public contract, persisted meaning, security/privacy
  boundary, production or external state, or destructive behavior;
- it does not weaken acceptance criteria or required verification; and
- a reasonable alternative would not materially change what reviewers believe
  they are approving.

The assumption and its consequence MUST be recorded in the pull request and
completion report. Prefer the smallest change that preserves future choices.

A contributor MUST stop and request a decision when missing information could
materially change any of the following:

- the observable outcome, acceptance criteria, milestone, or scope;
- compatibility, data meaning or loss, security, privacy, authorization, or
  compliance;
- an irreversible or destructive operation;
- production, third-party, or other externally visible state;
- meaningful cost, operational ownership, or deployment topology; or
- a choice between credible designs with materially different trade-offs.

The request MUST state the exact decision needed, the evidence already checked,
the viable options and consequences, and any work that can continue safely.
Agents MUST NOT bury a material choice in a diff, and MUST NOT halt on a trivial
choice that meets the proceed criteria.

## Issue timing

Issue-first is the preferred workflow, but issue-after is the traceability
fallback when implementation is requested without an existing issue.

- If an issue number is supplied, the contributor MUST follow that issue and
  its milestone, and the pull request MUST link it. A material mismatch follows
  the ambiguity and escalation rules.
- If no issue is supplied, the contributor MUST define the smallest coherent
  scope, perform the requested work, and include a complete proposed issue in
  the completion report. The proposed issue MUST describe the work actually
  performed, not an invented broader initiative.
- If repository permissions, branch protection, or the requester explicitly
  requires an issue before implementation, the contributor MUST stop after
  discovery and ask for or create the issue as authorized.
- Read-only investigation and scoping MAY occur before an issue exists.

Creating an issue, milestone, pull request, release, or other external record is
an externally visible write. A contributor MUST do so only when requested or
when the assigned workflow explicitly grants that authority. Otherwise, propose
the content without publishing it.

## Milestones

Milestone titles use:

~~~text
MNN - Short outcome
~~~

Numbering starts at M01 and uses two digits through M99. A published milestone
MUST NOT be reused or renumbered. The text after the number describes the
capability available when the milestone is complete, not a team activity.

Every issue MUST contain a Milestone line using the exact title. GitHub remains
the authoritative assignment after the milestone and issue have been created.
Before allocating a new milestone number, a contributor MUST refresh the
current GitHub milestone list. A collision MUST be resolved by choosing the
next available number and updating references, never by forcing conflicting
metadata through review.

## Required issue structure

Use an imperative title in specific domain language. Prefer “Expose impacted
tests for an API change” to “Implement impact service.” Avoid prefixes that
duplicate labels or milestones.

Every implementation task MUST use this body:

~~~markdown
**Milestone:** MNN - Outcome

## Goal

Describe the problem and observable result.

## Scope

- Included behavior and boundaries.

## Design decisions

- Important choices, assumptions, constraints, compatibility effects, and ADR
  links.

## Acceptance criteria

- [ ] Observable behavior and verification evidence.
- [ ] Relevant failure or negative behavior.
- [ ] Documentation and operational effects.

## Out of scope

- Explicit exclusions and deferred work.
~~~

Acceptance criteria MUST describe externally observable behavior or verifiable
invariants, not activities such as “create a class” or “write tests.” Known
limitations MUST be recorded rather than hidden.

## Pull-request-sized work

A pull request MUST:

- solve one problem or deliver one coherent vertical capability;
- keep the repository buildable and deployable;
- include the implementation, tests, documentation, contract changes, and
  migration required for that capability;
- be reviewable without depending on an unmerged speculative follow-up; and
- contain evidence for expected and important failure behavior.

Split work when it combines independent behavior, broad cleanup, dependency
upgrades, schema redesign, unrelated formatting, or several architectural
decisions. A change is not automatically too large because it touches several
layers; a thin vertical slice may legitimately include domain, application,
adapter, storage, and tests.

Placeholder abstractions, empty packages, unused ports, and future
configuration MUST NOT be created merely to make a roadmap look implemented.

## Development workflow

1. Select the issue workflow defined in [Issue timing](#issue-timing).
2. Create a focused branch when the surrounding workflow supports branches.
3. Add or update tests with the behavior.
4. Implement the smallest coherent solution.
5. Format and run the exact repository checks.
6. Review the complete diff for secrets, generated churn, accidental API
   changes, assumptions, and unrelated edits.
7. Update documentation, ADRs, migration notes, and troubleshooting material.
8. Open a linked pull request only when authorized, and include verification
   evidence.
9. Produce the canonical [Completion report](#completion-report).

Commit subjects SHOULD be clear and imperative. Generated output MUST be
reproducible and SHOULD be committed only when consumers require it without
running the generator.

## Verification and constrained environments

A contributor MUST run every applicable required repository check that the
current environment supports. Narrower checks MAY provide interim feedback,
but MUST NOT be presented as equivalent to the required suite.

When a required check cannot execute because of sandbox restrictions, missing
network access, unavailable credentials, absent services or fixtures, platform
limits, or another environmental constraint, the contributor MUST:

1. report the exact command or check as **not run**;
2. state the concrete blocking condition and any safe attempts made;
3. list the evidence from checks that did run;
4. state the residual risk and where or how the missing check should run; and
5. avoid changing the check merely to make the current environment pass.

A contributor MUST NOT report an unexecuted check as passed, fabricate output,
silently skip a check, or substitute a weaker check without labelling the
difference. A failing check MUST be reported as failed, even when the failure
appears unrelated. If an unrelated pre-existing failure is verified, identify
it separately with evidence.

Run `make fmt` before final verification and review its diff. `make validate`
is the required non-mutating Go acceptance suite; it builds the command, checks
formatting, runs pinned lint and static analysis, verifies module state, runs
ordinary and race-enabled tests, and scans reachable vulnerabilities. A
narrower target MAY provide interim feedback but MUST NOT be reported as the
complete suite.

The pinned tools and exact targets are defined in the root Makefile. Their first
run and the vulnerability database may require network access. When that access
is unavailable, follow the constrained-environment protocol above and report
the affected target as not run rather than weakening or silently omitting it.
Documentation structure and link checks remain manual evidence until replaced
by checked-in automation. Additional language workspaces MUST extend
`make validate` instead of requiring contributors to discover hidden checks.

## Documentation

Every long document MUST contain a `## TL;DR` section immediately after its
title and status metadata. A document is long when any of the following applies:

- it has 800 or more words;
- it has more than five second-level sections; or
- it is an architecture, end-to-end workflow, security, operational, or
  migration guide readers will consult selectively.

The TL;DR SHOULD state the purpose, intended reader, important decision or
outcome, and next action where one exists, preferably in three to seven bullets.
It does not replace detailed safety constraints.

Documentation follows the same review standard as code. Examples SHOULD be
executable or verified where practical. Stale comments and links MUST be
updated or removed in the same change that makes them inaccurate.

Architecture decisions use the template in
[docs/decisions/README.md](docs/decisions/README.md). Accepted ADRs are
historical records; supersede rather than rewrite them.

## Pull request description

A pull request MUST state:

- linked issue and milestone;
- problem and resulting behavior;
- boundaries, deliberate exclusions, assumptions, and unresolved questions;
- design and compatibility decisions;
- verification commands and evidence, including every check not run;
- migration, rollout, rollback, security, and operational considerations; and
- known limitations or follow-up work.

## Review checklist

- [ ] MUST: The issue belongs to a milestone and follows the required structure.
- [ ] MUST: The change is one coherent capability with explicit exclusions.
- [ ] MUST: Material assumptions and unresolved questions are visible.
- [ ] MUST: Dependencies point inward; infrastructure does not leak into domain
      policy.
- [ ] MUST: Patterns and abstractions solve demonstrated needs.
- [ ] MUST: Public contracts and persistence changes are compatible or have an
      approved evolution plan.
- [ ] MUST: Tests cover observable success and relevant negative/failure paths.
- [ ] MUST: Concurrency, resource ownership, idempotency, and retries are
      explicit where applicable.
- [ ] MUST: Security, privacy, and untrusted-input boundaries were reviewed.
- [ ] MUST: Public APIs and non-obvious invariants are documented.
- [ ] MUST: Long documentation has a useful TL;DR.
- [ ] MUST: Migrations satisfy the migration criteria and upgrade verification.
- [ ] MUST: Applicable checks passed, and unavailable checks are reported
      honestly under the constrained-environment protocol.
- [ ] MUST: No secrets, production data, machine-specific paths, or unrelated
      changes are included.
- [ ] MUST: An ADR exists when the decision meets the ADR threshold.

## Completion report

After every completed work item, the contributor MUST print:

1. the exact milestone in the form `MNN - Outcome`;
2. a proposed GitHub issue title;
3. the complete issue body from [Required issue structure](#required-issue-structure),
   matching the work actually performed;
4. assumptions, unresolved questions, and limitations; and
5. verification commands and outcomes, explicitly distinguishing **passed**,
   **failed**, and **not run**.

When an existing issue was supplied, the report SHOULD repeat or amend its
title and body so the delivered scope is auditable. When no issue was supplied,
the report provides the issue-after traceability record; it does not imply that
the issue was published.
