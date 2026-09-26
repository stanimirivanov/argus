# ADR-0012: Plan execution from reviewed repository bindings

- Status: Accepted
- Date: 2026-09-26
- Milestone: M05 - Functional API selection
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Group manifest decisions by stable test-repository identity and adapter.
- Keep immutable test revisions, stable CI group keys, and adapter commands out
  of the selection manifest.
- Require a reviewed binding for every manifest group and reject extra,
  missing, duplicate, or coordinate-mismatched bindings.
- Emit a flat, deterministic job array that CI systems can use directly as a
  matrix: selected jobs only when tests were selected, and a full-suite job for
  every group.
- Bind the generated plan and every resulting attempt to the SHA-256 of the
  canonical manifest.

## Context

An execution manifest can contain functional API tests from multiple
repositories and multiple framework adapters. The existing runner deliberately
executes only one repository/adapter/stage group so repository setup and
commands remain under CI control. Manually translating every manifest into
jobs is error-prone and does not scale to heterogeneous suites.

Selection evidence knows which stable tests should run, but it does not own the
immutable revision to check out from each test repository, a safe name for CI
artifacts and attempts, credentials, or executable commands. Letting the
manifest provide those values would turn catalog-controlled data into a code
execution authority. Conversely, requiring a hand-maintained job list can omit
the later control that keeps shadow-mode selection safe.

## Decision

Argus defines two Effect Schema-authored v1 contracts:

- functional API execution bindings contain a stable local group key, exact
  repository identity and observed coordinates, immutable test revision, and
  adapter ID; and
- a functional API execution plan contains canonical manifest identity and a
  flat list of runnable repository/adapter/stage jobs.

Bindings are reviewed configuration independent of one pull request. They do
not repeat a manifest digest. During one planning invocation Argus validates
the manifest and bindings together, requires their group sets and repository
coordinates to match exactly, computes the canonical manifest SHA-256, and
places that digest in the generated plan. This keeps configuration practical
while making the resulting jobs reproducible.

The grouping key is `(stable test-repository identity, adapter ID)`. A group
key is unique reviewed metadata used for matrix entries, attempt IDs, and
artifact names; it is not a test identity. Adapter IDs and group keys use the
bounded local-key syntax. Bindings with duplicate group keys or duplicate
repository/adapter identities are invalid.

The plan is a flat job array rather than a nested product so GitHub Actions and
other CI systems can consume it without reimplementing policy. Jobs are sorted
by group key with `selected` before `full-suite`. A selected job exists only
when that group contains at least one `RUN_REQUIRED` test. Every non-empty
group always has one full-suite job containing every candidate in the group.
The selected count cannot exceed the full-suite count, and jobs sharing a
group key must use identical repository, revision, and adapter metadata.

The plan contains no executable, shell fragment, secret, credential, token,
environment, or runner label. Reviewed CI configuration maps the adapter ID to
an explicit command and controls checkout, installation, isolation, network,
secrets, artifacts, ingestion, and failure gates. The existing runner filters
the original manifest for the exact planned repository, adapter, and stage and
still performs exact result correlation.

## Alternatives considered

### Put repository revisions and commands in the selection manifest

- Benefits: one self-contained document could launch all jobs.
- Costs and risks: selection or catalog data gains command-execution authority;
  revisions and environment policy become coupled to selection policy.
- Reason not selected: execution authority must remain reviewed CI
  configuration.

### Let CI discover groups independently

- Benefits: no additional contracts or planner command.
- Costs and risks: every CI integration reimplements grouping, ordering,
  fallback, and control-stage rules and can silently omit a control.
- Reason not selected: those are portable Argus invariants.

### Emit a nested group and stage structure

- Benefits: visually compact and closer to the conceptual hierarchy.
- Costs and risks: CI-specific expressions must flatten it and handle groups
  without selected tests.
- Reason not selected: a flat array is directly usable as a matrix include.

### Resolve a moving branch or default branch in Argus

- Benefits: less configuration for test revisions.
- Costs and risks: plans stop being reproducible and late jobs may execute
  different code.
- Reason not selected: bindings require an immutable full revision.

## Consequences

### Positive

- One manifest can drive multiple repositories and adapters deterministically.
- Every group retains its mandatory full-suite control job.
- CI command authority and secrets remain outside machine-produced selection
  evidence.
- Plan jobs can be consumed directly by a GitHub Actions matrix.
- Stable group keys make attempts and artifacts easier to correlate.

### Negative

- CI must resolve and supply an immutable revision for every test repository.
- Repository renames require reviewed binding and catalog-coordinate updates.
- A group with no selected tests has only a full-suite attempt, so the v1
  pairwise shadow report cannot represent that group by itself.

### Neutral or follow-up

- Aggregate plan-level shadow evaluation, including full-only groups, remains
  future work; full-suite execution remains authoritative meanwhile.
- Framework-specific adapters and installation stay repository-owned.
- Remote dispatch, queues, runners, and cancellation are not introduced.

## Compatibility and migration

This adds bindings and plan v1 contracts and a new command. Existing execution
manifest, adapter, attempt, and shadow-report v1 contracts are unchanged. No
database migration is required. Contract evolution follows versioned additive
or new-version rules rather than silent reinterpretation.

## Security and operations

Bindings and manifests are untrusted input at the CLI boundary and receive
structural plus semantic validation. Inputs are bounded to 16 MiB and group/job
counts to manifest limits. The planner performs no network, checkout, process,
database, or secret operation.

CI must map known adapter IDs to literal executable and argument vectors; it
must not interpolate an adapter ID into a shell command. Checkouts use the
exact revision from the validated plan and disable credential persistence where
supported. Full-suite jobs remain release authority during shadow mode.

## Validation

- Effect Schema fixtures verify bindings and plan structure and generated
  schema reproducibility.
- Go tests verify heterogeneous grouping, deterministic order, exact binding
  coverage, coordinate mismatch rejection, control-job presence, count
  invariants, and CLI output.
- Architecture tests keep planning free of contracts, processes, SQL, and CI
  implementation details.
- The reference GitHub Actions guide demonstrates matrix consumption while
  retaining explicit adapter-command mappings and immutable checkouts.
