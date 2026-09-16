# Repository working agreement

This file is the concise, tool-facing entry point for contributors and coding
agents. [CONTRIBUTING.md](CONTRIBUTING.md) is the canonical workflow policy.
Normative terms such as MUST, SHOULD, and MAY have the meanings defined there.

## Before changing anything

1. You MUST read [CONTRIBUTING.md](CONTRIBUTING.md), especially its
   [ambiguity](CONTRIBUTING.md#ambiguity-and-escalation),
   [issue-timing](CONTRIBUTING.md#issue-timing), and
   [verification](CONTRIBUTING.md#verification-and-constrained-environments)
   rules.
2. You MUST inspect the working tree and preserve pre-existing changes.
3. You MUST read the [product definition](docs/product/product-definition.md),
   [architecture overview](docs/architecture/overview.md), relevant material
   under [docs/development](docs/development), and accepted decisions in
   [docs/decisions](docs/decisions/README.md).
4. You MUST use the repository-local build and verification commands. You
   MUST NOT claim that an unavailable or unexecuted check passed.

If code, documentation, and an accepted decision disagree, you MUST NOT
silently pick one. Correct an obvious local error or propose a superseding ADR.
Ordinary missing requirements and implementation ambiguity follow the
escalation rules in CONTRIBUTING; they do not automatically require an ADR.

## Shape of work

- Each task and pull request MUST deliver one coherent, independently
  reviewable capability.
- Prefer a thin end-to-end slice over an unused layer or speculative framework.
- Unrelated refactoring, dependency updates, generated churn, and formatting
  MUST stay out of behavioral changes.
- Scope, exclusions, compatibility effects, risks, assumptions, and
  verification evidence MUST be explicit.
- An abstraction, service, queue, cache, database, or dependency MUST NOT be
  added without behavior in the current change that requires it.

## Architecture and implementation

- Organize around domain capabilities. Dependencies MUST point inward:
  adapters may depend on application and domain code; domain code MUST NOT
  depend on transports, persistence, frameworks, model providers, or vendors.
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
  domain. AI/model output MUST be treated as untrusted input.
- External calls MUST stay out of database transactions. Define idempotency,
  cancellation, timeout, retry, concurrency, and partial-failure behavior.
- Published APIs, events, schemas, manifests, persisted payloads, and
  prompts/evaluation contracts are compatibility boundaries.

Follow [engineering standards](docs/development/engineering-standards.md) and
[SQL migration criteria](docs/development/sql-migrations.md).

## Quality and documentation

- Test observable behavior at the lowest boundary that proves it. Defect fixes
  SHOULD begin with a failing regression test.
- Tests MUST be deterministic, isolated, parallel-safe, and independent of wall
  clock, network, locale, and execution order unless those are the subject.
- Exported/public APIs, invariants, units, side effects, ownership, concurrency,
  security boundaries, and failure semantics MUST be documented.
- Comments SHOULD explain why and constraints; they SHOULD NOT narrate syntax.
- Long documentation MUST include a TL;DR as defined in CONTRIBUTING.md.
- User-visible contracts, configuration, migrations, operational behavior, and
  troubleshooting documentation MUST change in the same pull request as code.
- Secrets, customer data, production evidence, machine-specific paths, and
  unreviewed generated binaries MUST NOT be committed.

## Completion

The single source of truth for completion output is
[Completion report](CONTRIBUTING.md#completion-report). Follow it after every
work item; do not infer a different report shape from this summary.

Planned milestones are listed in
[docs/roadmap/milestones.md](docs/roadmap/milestones.md).
