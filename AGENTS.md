# Repository agent guide

## Agent skills

Use `mattpocock/skills` as the primary workflow for planning, ticketing,
implementation, TDD, review, and pull requests. The skills take precedence where
contributor guidance covers the same steps. Follow the repository's technical
requirements, verification gates, and authorization boundaries.

### Issue tracker

Use GitHub Issues in `stanimirivanov/argus`. See [issue tracker configuration](docs/agents/issue-tracker.md).

### Triage labels

Use the default Matt Pocock label mapping. See [triage labels](docs/agents/triage-labels.md).

### Domain docs

Use a single root glossary and ADRs in `docs/adr/`. See [domain docs](docs/agents/domain.md).

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

Preserve unrelated work and follow [engineering standards](docs/development/engineering-standards.md) and
the task-specific routes in the [documentation map](docs/README.md). Database
changes additionally follow the
[SQL migration criteria](docs/development/sql-migrations.md).

## Quality and documentation

- Test observable behavior at the lowest boundary that proves it.
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

The [coding harness guide](docs/development/harness.md) maps repository guidance
to its executable feedback sensors and verification tiers.
When the host provides task scope and requests recorded verification, use the
[agent harness runner](tools/agent-harness/README.md) to inspect the declared
profile and record named offline checks. Its policy validation and limited
child environment do not enforce filesystem or network isolation; the agent
host owns those restrictions.

## Verification

Run `make verify` for local validation and `make validate` for complete
non-database acceptance. Apply the additional gates for the affected boundary,
including `make db-validate` for database changes. Report exact outcomes and
limitations under the [verification rules](CONTRIBUTING.md#verification-and-constrained-environments).
