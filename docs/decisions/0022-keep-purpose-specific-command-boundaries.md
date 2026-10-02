# ADR-0022: Keep purpose-specific command boundaries

- Status: Proposed
- Date: 2026-10-01
- Milestone: M10 - Production readiness
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Keep separate server, administrator, and CI-worker executables rather than
  introducing a public umbrella CLI or a CLI framework.
- Treat direct-database catalog, selection, and evidence commands as
  transitional clients; ordinary CI must not need PostgreSQL credentials.
- All executables expose top-level help and build identity before acquiring
  credentials or resources. Exit 0 means success, 1 an operational failure,
  and 2 invalid command usage.
- Keep application argument policy in CLI adapters and domain policy inward.

## Context

Argus has a server, a privileged migrator, CI-local workers, and several
database-backed commands used to prove early vertical slices. They are separate
trust and deployment roles, but their help, version, and exit behavior have
grown independently. Some commands treat help as a failure, and the server and
migrator previously ignored unexpected positional arguments. A single public
`argus` command would combine authorities before its client/API boundary is
designed.

## Decision

Retain purpose-specific executables:

- `control-plane` is the long-running server.
- `migrate` is the privileged schema administrator.
- `descriptor`, `plan-functional-api`, `run-functional-api`, proposal,
  validation, publication, and review-observation commands are CI/local
  workers with explicit inputs and outputs.
- `catalog`, `select`, `execution-evidence`, and `adaptation-evidence` are
  transitional direct-database clients. They are not the long-term CI access
  pattern; authenticated control-plane APIs will replace that authority.

Every executable accepts standalone leading `-h`/`--help` and
`-version`/`--version` without requiring configuration, credentials, a
database, or an adapter. Help describes the top-level invocation and role;
subcommand-specific help is not promised. Version prints the Go build module
version when available and VCS revision/modified state from Go build metadata,
with explicit development/unknown fallbacks. It never prints environment
values or credentials.

Exit 0 means successful work or metadata output; exit 1 means an operational,
configuration, policy, or dependency failure; exit 2 means invalid command
syntax. CLI adapters classify syntax errors explicitly rather than by parsing
error text. Executable roots report the classification and compose adapters;
they do not parse capability policy.

## Alternatives considered

### Consolidate into a public `argus` CLI now

- Benefits: One executable and potential shared generated help.
- Costs and risks: Merges server, migration, database, and CI-worker authority
  before authenticated APIs and distribution requirements are known.
- Reason not selected: Distinct trust roles and narrow machine-facing commands
  are currently more useful than a broad human-facing CLI.

### Adopt Cobra or another command framework

- Benefits: Subcommand help, completion, and generated reference material.
- Costs and risks: Adds a dependency and framework conventions for a command
  surface that is not yet a supported public product.
- Reason not selected: The standard `flag` package plus a small entry protocol
  meets the demonstrated need.

## Consequences

### Positive

- Help and version can be inspected safely without external state.
- CI can distinguish syntax mistakes from runtime failures consistently.
- Command trust roles are explicit without creating a new distribution unit.

### Negative

- Existing callers that inspect exact exit code 1 for usage errors will now
  observe code 2; callers that only require nonzero failure are unaffected.
- Direct-database commands remain until authenticated replacement APIs exist.
- Help is top-level; subcommand-specific help is a later product decision.

### Neutral or follow-up

- Do not add new direct-database commands. A future supported human CLI
  requires its own user, authorization, compatibility, and framework decision.

## Compatibility and migration

Existing successful command inputs and JSON output contracts are unchanged.
Unrecognized server/migrator arguments now fail with usage code 2 instead of
being silently ignored. Scripts depending on exact usage failure code should
accept 2. No binary names or adapter protocols change.

## Security and operations

Metadata handling precedes secret loading and external calls. The version
line contains only Go build version and VCS provenance. The migrator remains
separately deployable with elevated database authority; direct-database
clients must not be placed in ordinary CI with production credentials.

## Validation

Test metadata and exit classification without a configured database, exercise
representative command binaries on Windows and Linux, and run `make validate`.
