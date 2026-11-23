# ADR-0008: Derive capability impact from OpenAPI operations

- Status: Accepted
- Date: 2026-09-18
- Milestone: M04 - Change impact
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Compare OpenAPI 3 documents fetched at the immutable change-set revisions.
- Use libopenapi for standards-aware comparison and Argus-owned operation
  fingerprints for capability-level reasons.
- Map operations only through explicit x-argus-capabilities arrays.
- Persist unmapped operations and partial-analysis warnings; never turn missing
  evidence into an empty complete impact.
- Keep parsing behind a provider-neutral analyzer port and expose an
  Effect-authored argus.dev/capability-impact/v1 contract.

## Context

M04 must turn trusted changed-file evidence into affected capabilities without
making filename or patch-text guesses look authoritative. OpenAPI operations
are a useful first semantic source, but document changes can be indirect through
local schema references. Documents can also be truncated, invalid, or split
across external references. The result must survive retries and process
restarts and remain inspectable before M05 consumes it for test selection.

## Decision

The GitHub adapter loads candidate YAML or JSON documents through the repository
contents API at the exact base and head Git digests. Responses are limited to
5 MiB. The OpenAPI adapter recognizes OpenAPI 3.0, 3.1, and 3.2, uses
libopenapi v0.38.7 for semantic document and breaking-change counts, and
fingerprints each HTTP operation together with path/root context and recursively
referenced same-document values.

An operation declares repository-scoped capabilities through
x-argus-capabilities, an array of catalog local keys. Argus unions old and new
mappings for modified operations so a mapping removal does not erase the review
reason. Changed operations without a mapping remain explicit evidence with an
empty capability list. Unsupported external references, omitted files, and
bounded discovery produce a partial assessment with warnings. Downstream
selection must broaden or abstain for partial or unmapped evidence.

The impact application owns analyzer and store ports. A workflow service
composes trusted ingestion and assessment; the webhook is acknowledged only
after both records are durable. PostgreSQL claims one immutable assessment per
change set, stores typed document, operation, capability, and warning rows, and
uses a canonical SHA-256 to distinguish exact retries from conflicts. A local
catalog command returns the versioned explainability contract.

## Alternatives considered

### Diff patches or operation IDs only

- Benefits: Smaller implementation and no parser dependency.
- Costs and risks: Misses referenced schema changes and confuses formatting
  edits with semantic changes.
- Reason not selected: It cannot provide reliable semantic evidence.

### Infer capabilities from paths or operation names

- Benefits: No source annotation required.
- Costs and risks: Repository naming conventions become hidden policy and can
  silently over- or under-select tests.
- Reason not selected: Explicit mappings are reviewable and stable.

### Treat unsupported evidence as no impact

- Benefits: Simpler consumer behavior.
- Costs and risks: Creates unsafe false negatives.
- Reason not selected: Absence of understood evidence is not negative evidence.

## Consequences

### Positive

- Schema-only changes can identify the operations and capabilities they affect.
- Every selected capability has document and operation evidence.
- Unmapped and incomplete cases remain visible for conservative policy.
- Domain and application packages do not depend on an OpenAPI library.

### Negative

- First-time processing reads each candidate at up to two revisions.
- Multi-file and remote-reference OpenAPI descriptions initially yield partial
  results.
- Analyzer-version changes require an explicit compatibility and replay plan.

### Neutral or follow-up

- M05 will bind capability impact to cataloged functional API tests.
- Configuration, source-code, and test-file analyzers remain later M04 inputs.
- A future analyzer can support trusted multi-file bundles behind the same port.

## Compatibility and migration

The public addition is argus.dev/capability-impact/v1. The additive migration
creates impact tables without rewriting existing data. Existing change sets are
not backfilled automatically. Rolling application code back leaves unused
impact evidence intact.

## Security and operations

Document fetches use the existing read-scoped GitHub credential, immutable
digests, HTTPS policy, and response limits. Parser remote and filesystem
reference lookup remain disabled. The analyzer does not execute repository
content. Database writes remain short and occur after provider I/O.

## Validation

Effect Schema generation and Go transport validation cover the wire boundary.
Unit tests cover immutable document loading, referenced-schema changes,
explicit and missing mappings, retry behavior, and import direction.
PostgreSQL integration tests cover migration, round trip, exact retry,
conflict, and restart behavior. Repository acceptance remains make validate
plus make db-validate.
