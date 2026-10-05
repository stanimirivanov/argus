# Argus documentation map

## TL;DR

- Start with [CONTRIBUTING.md](../CONTRIBUTING.md) for workflow policy, then
  use this page to load only the sources relevant to the task.
- Product behavior belongs in the product definition; durable technical choices
  belong in ADRs; current structure belongs in the architecture overview.
- Development guides describe how to work in a particular area. They do not
  override product policy or accepted decisions.
- Update this map when a canonical document moves, a new documentation area is
  introduced, or a recurring task needs an explicit route.

## Purpose and authority

This page is the progressive-disclosure entry point for contributors and
coding agents. It routes readers to canonical sources; it is not another copy
of their policy. [CONTRIBUTING.md](../CONTRIBUTING.md) defines source priority,
ambiguity handling, issue timing, verification, and completion reporting.
[AGENTS.md](../AGENTS.md) is the concise tool-facing entry point.

Read the rows that match the work. Do not bulk-read every guide or ADR unless
the change actually spans those concerns. When a change affects product
behavior or a dependency boundary, the product definition and architecture
overview are always relevant.

## Route by task

| Task or changed area | Read before changing it | Verify or consult as needed |
|:--|:--|:--|
| Product scope, test taxonomy, decision outcomes, or safety | [Product definition](product/product-definition.md) | [Research landscape](research/landscape.md), relevant [ADRs](decisions/README.md) |
| Capability ownership, dependency direction, deployment boundary, or cross-product reuse | [Architecture overview](architecture/overview.md), [engineering standards](development/engineering-standards.md) | [ADR index](decisions/README.md), `make architecture-check` |
| Go or TypeScript implementation and tests | [Engineering standards](development/engineering-standards.md) | Relevant capability documentation and `make verify` |
| Effect Schema, generated JSON Schema, wire contracts, or compatibility fixtures | [Contract workspace](../contracts/README.md), [ADR-0003](decisions/0003-use-effect-schema-at-contract-boundaries.md) | Relevant integration guide, `make check` |
| PostgreSQL schema, queries, migrations, or persisted meaning | [SQL migration criteria](development/sql-migrations.md), [PostgreSQL guide](development/postgresql.md), [ADR-0004](decisions/0004-use-postgresql-and-embedded-forward-migrations.md) | Relevant evidence ADR, `make db-validate` |
| GitHub change ingestion or OpenAPI impact | [Architecture overview](architecture/overview.md), [ADR-0007](decisions/0007-ingest-github-changes-as-bounded-immutable-evidence.md), [ADR-0008](decisions/0008-derive-capability-impact-from-openapi-operations.md) | [Security policy](../SECURITY.md), capability tests |
| Opt-in durable change-workflow evaluation | [ADR-0028](decisions/0028-evaluate-embedded-dbos-workflows-for-change-processing.md), [PostgreSQL guide](development/postgresql.md) | [Local start](development/local-start.md), `make db-validate` |
| Test selection and execution planning | [ADR-0009](decisions/0009-start-functional-api-selection-with-deterministic-safe-fallbacks.md), [ADR-0012](decisions/0012-plan-execution-from-reviewed-repository-bindings.md), [ADR-0013](decisions/0013-evaluate-selection-across-complete-execution-plans.md) | [GitHub Actions integration](integrations/github-actions-functional-api.md) |
| Functional UI capability selection | [ADR-0025](decisions/0025-select-browser-tests-only-for-fully-covered-api-changes.md), [Architecture overview](architecture/overview.md) | [Functional UI guide](integrations/functional-ui-selection.md), M07 roadmap |
| Functional UI component-root selection | [ADR-0027](decisions/0027-opt-in-browser-selection-from-declared-component-roots.md), [Architecture overview](architecture/overview.md) | [Functional UI guide](integrations/functional-ui-selection.md), M07 roadmap |
| Playwright UI catalog conformance | [ADR-0026](decisions/0026-check-playwright-inventory-against-declared-tests.md), [Product definition](product/product-definition.md) | [Functional UI guide](integrations/functional-ui-selection.md), M07 roadmap |
| Functional API adapter execution | [Adapter protocol](integrations/functional-api-adapters.md), [ADR-0010](decisions/0010-use-a-bounded-process-protocol-for-functional-api-execution.md) | [GitHub Actions integration](integrations/github-actions-functional-api.md), adapter conformance tests |
| Repair proposal, validation, publication, or review evidence | [Adaptation protocol](integrations/functional-api-adaptation.md), relevant ADRs 0014 through 0019 in the [ADR index](decisions/README.md) | [Security policy](../SECURITY.md), adaptation fixtures and tests |
| Build, CI, repository policy, agent guidance, or developer experience | [Harness guide](development/harness.md), [developer quickstart](development/developer-quickstart.md) | [Dependency policy](development/dependency-policy.md), `make docs-check`, `make verify`, `make validate` |
| Start the local control plane or prepare the DBOS evaluation | [Local start](development/local-start.md), [developer quickstart](development/developer-quickstart.md) | [PostgreSQL guide](development/postgresql.md), [ADR-0028](decisions/0028-evaluate-embedded-dbos-workflows-for-change-processing.md) |
| Dependency admission, upgrades, licensing, or vulnerability response | [Dependency policy](development/dependency-policy.md) | [Security policy](../SECURITY.md), `make supply-chain` |
| Milestone or issue planning | [Milestones](roadmap/milestones.md), [CONTRIBUTING.md](../CONTRIBUTING.md) | Relevant product and architecture sources |
| Research or competitive claims | [Research landscape](research/landscape.md) | Primary sources recorded with the claim |
| Vulnerability reporting or security-sensitive behavior | [Security policy](../SECURITY.md) | Product safety rules, relevant ADRs and integration guides |

## Canonical collections

- [Product](product/product-definition.md) defines supported outcomes, test
  taxonomy, adaptation classes, safety gates, and non-goals.
- [Architecture](architecture/overview.md) describes current capabilities,
  information flow, trust boundaries, and cross-product relationships.
- [Development](development/) contains the engineering, environment,
  dependency, database, migration, and harness guides.
- [Integrations](integrations/) contains executable protocols and CI-facing
  workflows for external adapters.
- [Decisions](decisions/README.md) records durable choices and their lifecycle.
- [Roadmap](roadmap/milestones.md) groups delivery into outcome milestones and
  coherent pull-request slices.
- [Research](research/landscape.md) records evidence, alternatives, and open
  questions; it is not normative product policy.
- [Security](../SECURITY.md) defines vulnerability reporting and contribution
  expectations. [CODE_OF_CONDUCT.md](../CODE_OF_CONDUCT.md) defines community
  behavior.
- [Proposal decomposition](proposal.md) is a provenance record. Follow its
  canonical destination links instead of treating it as current policy.

## Keeping the map useful

A change MUST update this page when it adds, renames, moves, or removes a
canonical source or creates a recurring task that lacks a clear route. Avoid
listing implementation details that will drift quickly. `make docs-check`
checks documentation structure and repository-local links; semantic ownership
and stale guidance still require review.
