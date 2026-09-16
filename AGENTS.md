# Repository working agreement

This file is the concise, tool-facing entry point for contributors and coding
agents. Detailed rationale lives in the linked guides.

## Read before changing code

1. Read [CONTRIBUTING.md](CONTRIBUTING.md).
2. Read the relevant material under [docs/development](docs/development).
3. Read accepted decisions in [docs/decisions](docs/decisions/README.md).
4. Follow the repository-local build and verification commands once they are
   defined. Do not invent substitutes that weaken the checks.

If code, documentation, and an accepted decision disagree, do not silently pick
one. Correct an obvious local error or propose a superseding ADR.

## Shape of work

- Each task and pull request delivers one coherent, independently reviewable
  capability.
- Prefer a thin end-to-end slice over an unused layer or speculative framework.
- Keep unrelated refactoring, dependency updates, generated churn, and
  formatting out of behavioral changes.
- Make scope, exclusions, compatibility effects, risks, and verification
  evidence explicit.
- Do not add an abstraction, service, queue, cache, database, or dependency
  without behavior in the current change that requires it.

## Architecture and implementation

- Organize around domain capabilities. Dependencies point inward: adapters may
  depend on application and domain code; domain code does not depend on
  transports, persistence, frameworks, model providers, or vendors.
- Keep transport DTOs, persisted records, event envelopes, model output, and
  domain values distinct when they have different invariants or evolution.
- Use SOLID and established patterns as reasoning tools, not quotas. Prefer
  cohesion, explicit ownership, composition, small stable interfaces, and the
  simplest design that preserves the required boundary.
- Put interfaces at the boundary that consumes them. Do not create an interface
  solely to mirror every concrete type.
- Model meaningful identifiers, revisions, states, risks, and units explicitly.
  Make invalid states difficult to construct.
- Validate untrusted input at adapters and enforce business invariants in the
  domain. AI/model output is untrusted input.
- Keep external calls out of database transactions. Define idempotency,
  cancellation, timeout, retry, concurrency, and partial-failure behavior.
- Treat published APIs, events, schemas, manifests, persisted payloads, and
  prompts/evaluation contracts as compatibility boundaries.

Follow [engineering standards](docs/development/engineering-standards.md) and
[SQL migration criteria](docs/development/sql-migrations.md).

## Quality and documentation

- Test observable behavior at the lowest boundary that proves it. Defect fixes
  begin with a failing regression test.
- Cover relevant rejection, duplicate, stale-version, timeout, retry,
  cancellation, authorization, and partial-failure behavior.
- Tests are deterministic, isolated, parallel-safe, and independent of wall
  clock, network, locale, and execution order unless those are the subject.
- Document exported/public APIs, invariants, units, side effects, ownership,
  concurrency, security boundaries, and failure semantics.
- Comments explain why and constraints; they do not narrate syntax.
- Long documentation includes a TL;DR as defined in CONTRIBUTING.md.
- User-visible contracts, configuration, migrations, operational behavior, and
  troubleshooting documentation change in the same pull request as the code.
- Never commit secrets, customer data, production evidence, machine-specific
  paths, or unreviewed generated binaries.

## Required completion report

After completing each work item, report:

1. the milestone using the form MNN - Outcome;
2. a GitHub issue title;
3. an issue body with Goal, Scope, Design decisions, Acceptance criteria, and
   Out of scope;
4. verification performed and any remaining limitations.

The issue and pull request rules are defined in
[CONTRIBUTING.md](CONTRIBUTING.md). Planned milestones are listed in
[docs/roadmap/milestones.md](docs/roadmap/milestones.md).
