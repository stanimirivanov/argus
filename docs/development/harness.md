# Argus coding harness

## TL;DR

- The harness combines feed-forward guidance with deterministic feedback; no
  single guide, linter, or test is sufficient by itself.
- Use [the documentation map](../README.md) to load only relevant policy and
  use `make verify` for the fast, network-independent inner loop after
  `make bootstrap`.
- `make validate` remains the complete non-database acceptance suite;
  database-changing work additionally runs `make db-validate`.
- Recurring review findings SHOULD become clearer canonical guidance, an
  early deterministic sensor, or an explicit documented exception.
- Multi-PR work MAY use a versioned execution plan for technical sequencing;
  GitHub issues and milestones remain authoritative for delivery state.

## Purpose

The coding harness makes the repository legible and self-checking for human
contributors and coding agents. Feed-forward guides explain intent and
constraints before a change. Feedback sensors detect violations and behavioral
regressions after each edit. The shortest useful loop runs the cheapest
relevant sensors first, while the complete acceptance suite remains available
through one stable command.

This guide inventories the harness. It does not replace the canonical policy
linked below or the exact implementation in the root [Makefile](../../Makefile)
and CI workflows.

## Cost and timing tiers

| Tier | Intended timing | Characteristics |
|:--|:--|:--|
| T0 — route | Before editing | Read-only, immediate, and selected by task rather than loaded wholesale. |
| T1 — inner loop | During editing | Fast and deterministic; no network after a successful bootstrap. Run focused checks, then `make verify`. |
| T2 — acceptance | Before handoff or pull request | Broader CPU or supply-chain cost. Run `make validate`; record every unavailable check. |
| T3 — specialized | When the affected boundary requires it | Needs a service, credentials, a platform, or scheduled capacity, such as PostgreSQL integration or longer fuzzing. |

Cost tiers describe when feedback belongs, not permission to skip applicable
checks. [The constrained-environment protocol](../../CONTRIBUTING.md#verification-and-constrained-environments)
always applies.

## Feed-forward guides

| Guide | What it supplies | Load when | Policy owner |
|:--|:--|:--|:--|
| [AGENTS.md](../../AGENTS.md) | Concise tool-facing working agreement and links to canonical policy | Every task | Contributor workflow |
| [CONTRIBUTING.md](../../CONTRIBUTING.md) | Normative workflow, ambiguity, issue timing, verification, and completion rules | Every task | Contributor workflow |
| [Documentation map](../README.md) | Progressive task-to-source routing | Every task | Documentation architecture |
| [Product definition](../product/product-definition.md) | Outcomes, taxonomy, decision semantics, and safety gates | Product behavior or scope | Product |
| [Architecture overview](../architecture/overview.md) | Capability ownership, dependency direction, information flow, and trust boundaries | Code or boundary changes | Architecture |
| [Engineering standards](engineering-standards.md) | Language, design, testing, documentation, and operational practices | Implementation work | Engineering |
| [Developer quickstart](developer-quickstart.md) | Supported environments, setup, daily loop, and troubleshooting | Setup or tool failure | Developer experience |
| [Dependency policy](dependency-policy.md) | Admission, update, license, and vulnerability rules | Dependency or tool changes | Supply chain |
| [SQL migration criteria](sql-migrations.md) and [PostgreSQL guide](postgresql.md) | Persisted-data invariants and database operations | Database work | Persistence |
| [Integration guides](../integrations/) | CI-local protocols, security boundaries, and operational workflows | Adapter or integration work | Owning capability |
| [ADR index](../decisions/README.md) | Accepted durable decisions and supersession history | Decisions relevant to the task | Architecture |
| [Milestones](../roadmap/milestones.md) | Outcome sequence and coherent delivery slices | Planning and issue drafting | Product planning |
| [Security policy](../../SECURITY.md) | Reporting and security expectations | Security-sensitive work | Security |

Guidance SHOULD state intent, boundaries, and where its assertions are proved.
Avoid copying the same normative rule into multiple files. A concise entry
point links to the canonical rule instead.

## Feedback sensors and command surface

The root Makefile is the executable source of truth. Contributors MAY use a
focused sensor while editing, but MUST run the applicable aggregate before
handoff.

| Command or sensor | What it proves | Tier and dependencies | Ownership |
|:--|:--|:--|:--|
| `make bootstrap` | Pinned Go, Node, and quality dependencies are available | Setup actuator; network is normally required on first use | Developer experience and dependency policy |
| `make fmt` | Applies repository-owned Go and TypeScript formatting | T1; mutating, so inspect its diff | Language workspaces |
| `make docs-check` | Documentation shape and repository-local links satisfy mechanical policy | T1; checked-out repository and Go toolchain | Documentation architecture |
| `make architecture-check` | Production packages follow the declared capability and hexagonal dependency matrix | T1; Go source tree | Architecture and relevant ADRs |
| `make check` | Generated contracts, formatting, type checking, static analysis, workflows, and module state are coherent | T1; bootstrapped Go and Node workspaces | Language, contract, and CI owners |
| `make test` | Deterministic ordinary Go and TypeScript behavior passes without cached Go results | T1; bootstrapped workspaces | Changed capabilities |
| `make verify` | Build, repository checks, and ordinary tests pass as the closed inner loop | T1; network-independent after bootstrap | Repository maintainers |
| `make race` | Go tests pass with the race detector | T2; supported compiler/platform and more CPU time | Go capability owners |
| `make fuzz-smoke` | Bounded native fuzz campaigns exercise webhook payloads, catalog cursors, OpenAPI documents, process results, and source paths | T3; Go fuzzing support and additional CPU time | Untrusted-input boundary owners |
| `make vuln` | Go and production Node dependencies have no unaccepted known vulnerability | T2; network or current vulnerability caches | Security and dependency policy |
| `make license` | Runtime dependencies satisfy the license allowlist | T2; bootstrapped dependency graphs | Dependency policy |
| `make supply-chain` | Vulnerability and license sensors pass together | T2; same dependencies as `vuln` and `license` | Security and dependency policy |
| `make validate` | The complete non-database acceptance aggregate passes, including verification, race, and supply-chain sensors | T2; bootstrapped workspaces and vulnerability data | Repository maintainers |
| `make db-validate` | Migrations and PostgreSQL integration behavior pass against an isolated test database | T3; explicitly configured loopback PostgreSQL 17 | Persistence and affected capabilities |
| `make dbos-evaluate` | Durable webhook safety, real two-SDK compatibility hazards, measured overhead and guarded test-only retention | T3; Docker and pinned Go/tool dependencies; self-provisioned database and synthetic GitHub | Change workflow and ADR-0028; [verdict](dbos-evaluation-verdict.md) |
| CI platform jobs | The checked-in command surface behaves on supported Linux and Windows environments | T2/T3; hosted runners and network | Repository maintainers |

`make build` proves commands compile without writing repository artifacts.
`make binaries` is the explicit artifact-producing target and writes ignored
executables under `bin/`. Additional workspaces MUST join `make verify` and
`make validate`; contributors MUST NOT be expected to discover hidden checks.

The fuzz targets' checked-in seed cases execute during ordinary `go test`, so
`make verify` and `make validate` gate their deterministic regression behavior.
`make fuzz-smoke` additionally runs short mutation campaigns locally. Longer
targeted campaigns MAY be run with `go test -run '^$' -fuzz '^FuzzName$'
-fuzztime=...` in the owning package; triage a failure by keeping the generated
corpus file as a reviewed regression fixture. Mutation campaigns are not a CI
merge gate because their runtime and findings depend on generated inputs.

## Steering loop

When a review finding or escaped defect recurs, the owning contributor SHOULD
classify it and close the cheapest reliable loop:

1. If intent is unclear, improve the narrow canonical guide and its route.
2. If a structural rule is deterministic, add or improve an actionable sensor
   at the earliest useful tier.
3. If observable behavior regressed, add a focused fixture or test independent
   of the implementation being corrected.
4. If automation would be noisy or misleading, record the accepted exception,
   owner, and review trigger instead of adding a weak gate.

Sensor failures SHOULD identify the violated rule, affected location, allowed
shape, likely correction, and canonical guidance. A sensor MUST NOT silently
rewrite expected behavior, weaken an invariant, depend on ambient global tools,
or report an unavailable check as passed.

Harness changes require the same review as product changes. Review false
positives, repeated suppressions, runtime, flaky retries, stale documentation,
and escaped findings. Promote a new check to a blocking gate only after it is
deterministic, actionable, and owned.

## Multi-PR execution plans

Use a versioned execution plan when one approved outcome requires several
dependent pull requests, especially for migrations, compatibility transitions,
or architectural refactoring. The first implementation pull request SHOULD add
the plan under `docs/development/plans/` using a short kebab-case name. Creating
the directory is part of that first plan-bearing change; do not add empty
placeholder plans.

An execution plan SHOULD contain:

- the outcome, invariants, exclusions, and relevant ADRs;
- the ordered pull-request slices and dependencies between them;
- compatibility, migration, rollout, rollback, and security considerations;
- verification and promotion evidence for each stage; and
- decisions or discoveries that changed the sequence.

Each slice MUST remain independently reviewable and leave the repository
buildable. Update the plan in the pull request that changes the technical
sequence. The plan MUST NOT become a second issue tracker: GitHub issues and
milestones remain authoritative for assignment and delivery status, while ADRs
remain authoritative for durable decisions.

## Extending the harness

Add a guide or sensor only when it closes a demonstrated gap. Prefer extending
an existing command, checker, or canonical document over creating a parallel
tool. New sensors MUST be cross-platform where `make verify` is cross-platform,
pin their dependencies through repository-owned configuration, include tests
for failure diagnostics, and state which aggregate command owns them.

After a pattern proves identical across Argus, Perfeng, or Argus SRE, it MAY be
extracted behind a versioned shared tool or template. Product definitions,
architecture maps, acceptance policy, and repository-specific routes remain in
the repository that owns them.
