# ADR-0009: Start functional API selection with deterministic safe fallbacks

- Status: Accepted
- Date: 2026-09-18
- Milestone: M05 - Functional API selection
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Select functional API tests from explicit capability mappings in the approved
  base-revision catalog.
- Require directly mapped tests now and explain every test omitted from the
  early stage.
- Require omitted tests in a later full-suite control.
- Run every functional API candidate when impact is partial, empty, or unmapped.
- Publish deterministic decisions as argus.dev/execution-manifest/v1.

## Context

M04 produces immutable OpenAPI capability impact, while M03 stores stable test
identities and explicit capability mappings. M05 needs the first useful bridge
between them. The initial selector must reduce an early-stage test set without
allowing incomplete semantic evidence to create a false negative or making an
omission disappear from later validation.

The repository does not yet have historical failure calibration, duration
budgets, execution attempts, or adopted release-gate authority. The first
policy must therefore be deterministic, inspectable, and safe for shadow-mode
use.

## Decision

Argus reads a persisted capability-impact assessment by verified delivery
identity. It derives the catalog key from the same source repository and the
change's immutable base revision, using an explicitly versioned repository
descriptor. The base snapshot is the approved mapping authority for this first
policy; a pull request cannot silently redefine the tests used to assess itself.

For a complete assessment where every changed operation has an explicit
capability mapping, the selector:

- marks functional API tests sharing an affected capability as RUN_REQUIRED;
- marks other functional API tests as SKIP_FOR_NOW for the early stage;
- requires every skipped test in a later full-suite control;
- records stable machine-readable reasons for inclusion and omission; and
- reports affected capabilities with no mapped functional API test.

The selector switches to fallback mode and marks every functional API candidate
RUN_REQUIRED when the assessment is partial, contains no OpenAPI documents,
contains a changed document without operation evidence, contains an unmapped
operation, or produces no affected capability. Candidate and catalog scan
bounds fail closed rather than returning a partial manifest.

The result is the Effect-authored argus.dev/execution-manifest/v1 contract. It
contains immutable change, analyzer, catalog, and policy provenance. The local
select command emits JSON for CI consumers. This slice generates a manifest but
does not execute tests or make the subset a release authority.

## Alternatives considered

### Skip tests when evidence is absent

- Benefits: Maximum immediate reduction.
- Costs and risks: Missing or unsupported evidence becomes an unsafe false
  negative.
- Reason not selected: Missing evidence is not negative evidence.

### Use the pull-request head catalog

- Benefits: New tests and mappings can participate immediately.
- Costs and risks: A change can alter the policy inputs used to assess itself,
  and the head descriptor may not yet be trusted or available.
- Reason not selected: The initial policy uses the approved base snapshot.

### Return only selected tests

- Benefits: Smaller manifest.
- Costs and risks: Omission reasons and remaining-suite obligations disappear.
- Reason not selected: Every candidate needs an auditable decision.

### Add predictive ranking now

- Benefits: Potentially smaller and faster selections.
- Costs and risks: No local chronological dataset, calibration, or promotion
  evidence exists yet.
- Reason not selected: Predictive optimization remains M08 scope.

## Consequences

### Positive

- M04 evidence now produces a directly consumable functional API manifest.
- Inclusion, omission, fallback, provenance, and coverage gaps are explicit.
- The same immutable inputs produce the same ordered output.
- Conservative fallback prevents incomplete analysis from reducing coverage.

### Negative

- Non-OpenAPI changes initially run the entire functional API candidate set.
- The approved base catalog must exist before selection.
- Catalogs beyond the declared scan or decision bounds fail closed.

### Neutral or follow-up

- A reference CI adapter will consume the manifest in a later M05 slice.
- Execution attempts, normalized results, and remaining/full-suite comparison
  still need durable contracts and storage.
- Must-run pins, exclusions, criticality, budgets, and historical ranking will
  extend the policy under new version identifiers.

## Compatibility and migration

The change adds an output-only execution-manifest v1 contract and a local
command. It does not alter existing contracts or database schema. Consumers
must reject unsupported API or policy versions rather than guessing semantics.

## Security and operations

Selection reads immutable PostgreSQL evidence and does not execute repository
content. Database configuration remains environment-only. Bounded catalog
scans and manifest sizes prevent unbounded memory use. Errors fail closed and
do not emit a partial manifest.

## Validation

Effect Schema fixtures verify accepted and rejected wire documents. Go tests
cover targeted selection, explicit omissions, uncovered capabilities,
conservative fallback, catalog pagination and family filtering, CLI validation,
canonical ordering, and architecture boundaries. Repository acceptance remains
make validate.
