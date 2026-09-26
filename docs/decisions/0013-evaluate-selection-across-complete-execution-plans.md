# ADR-0013: Evaluate selection across complete execution plans

- Status: Accepted
- Date: 2026-09-26
- Milestone: M05 - Functional API selection
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Evaluate every repository/adapter group in one execution plan, including
  groups with no selected-stage attempt.
- Bind planned groups to explicit immutable attempt IDs and reject incomplete,
  additional, duplicate, or stage-inconsistent bindings.
- Validate every loaded attempt against the plan's manifest, repository,
  revision, adapter, stage, and expected test count before aggregation.
- Compute failure recall across all full-suite failures; a failure in a
  full-only group is a real `not-selected` miss.
- Record the canonical execution-plan digest in the report while retaining the
  pairwise report for focused diagnosis.

## Context

The first shadow report compares one explicitly named selected attempt with one
compatible full-suite attempt. This is sufficient for a repository/adapter
group that contains at least one selected test. The heterogeneous execution
planner, however, deliberately omits an empty selected job. A group in which
all tests were deferred therefore has only a full-suite attempt and cannot be
represented by the pairwise contract.

Ignoring those groups would bias aggregate failure recall upward: a failure in
a full-only group is precisely a failure that early selection did not catch.
CI also needs one reproducible report for the complete plan rather than an
unstructured collection of whichever pairs happened to be available.

## Decision

Argus adds two Effect Schema-authored v1 contracts:

- execution-plan attempt bindings name the explicit selected attempt, when the
  plan contains a selected job, and the mandatory full-suite attempt for every
  group; and
- a selection-plan shadow report records the canonical plan and manifest
  identities, aggregate metrics, and deterministic per-group evidence.

Attempt bindings are execution observations, not planning authority. They
contain group keys and attempt IDs only. They do not repeat repository,
revision, adapter, stage, test count, plan digest, commands, or credentials.
The evaluator requires their group set and selected-attempt presence to match
the validated plan exactly. It rejects reuse of one attempt ID in multiple
positions.

The evaluator loads immutable attempts by explicit ID. Every attempt must be a
valid stored execution attempt and match its planned job's manifest,
repository identity and coordinates, revision, adapter, stage, and exact test
count. A selected/full pair must additionally use the same adapter version and
the full-suite results must be a superset of selected results. Missing or
incompatible evidence fails the whole evaluation; Argus does not emit a
partial aggregate report.

For a group with selected execution, existing pairwise comparison semantics
are reused. For a full-only group, selected counts and duration are zero and
every failing or errored full-suite test is a `not-selected` miss. Aggregate
failure recall is:

~~~text
caught full-suite failures / all full-suite failures
~~~

It is `null` when no full-suite test failed. Per-group and aggregate durations
sum normalized per-test durations. They measure test work represented by the
attempts, not matrix wall-clock time, runner billing, or critical-path latency.

The report includes the SHA-256 of canonical execution-plan v1 JSON. Attempt
bindings omit that digest because the evaluator receives the plan directly and
validates every referenced attempt against its complete job provenance. This
keeps CI binding generation practical without weakening reproducibility.

The existing pairwise `selection-shadow-report/v1` remains supported for
single-group drill-down and compatibility. It is not silently reinterpreted.

## Alternatives considered

### Treat full-only groups as outside the recall denominator

- Benefits: the existing pairwise report could be aggregated unchanged.
- Costs and risks: reports would hide exactly the failures produced by a group
  selection omitted entirely.
- Reason not selected: it creates an optimistically biased safety metric.

### Create synthetic empty selected attempts

- Benefits: every group would fit the pairwise contract.
- Costs and risks: an attempt would claim execution that never happened, and
  existing attempt invariants require at least one normalized result.
- Reason not selected: evidence records must describe real execution.

### Discover attempts by prefix, recency, or manifest digest

- Benefits: CI would not need an attempt-binding document.
- Costs and risks: concurrent reruns and retries make implicit selection
  ambiguous and non-reproducible.
- Reason not selected: all evidence inputs remain explicit immutable IDs.

### Persist aggregate reports immediately

- Benefits: derived reports could be queried without recomputation.
- Costs and risks: introduces schema evolution, idempotency, and retention
  behavior before there is a consumer that requires durable derived state.
- Reason not selected: immutable attempts and plans are sufficient to derive
  the report deterministically on demand.

## Consequences

### Positive

- Failure recall includes every planned control failure.
- Full-only groups are visible instead of silently disappearing.
- CI receives one versioned report for the entire heterogeneous plan.
- Plan/job provenance prevents unrelated attempts from contaminating metrics.
- Existing pairwise consumers remain compatible.

### Negative

- CI must construct a small explicit attempt-binding document after execution.
- Evaluation waits until every planned full-suite attempt is durably present.
- Summed test duration does not describe parallel workflow latency.

### Neutral or follow-up

- Reports remain derived on demand and are not stored separately.
- Promotion thresholds and historical trend evaluation belong to M08.
- Artifact availability and checksum verification remain separate concerns.

## Compatibility and migration

This adds new v1 contracts and a new `plan-shadow-report` subcommand. Existing
execution plan, execution attempt, and pairwise shadow-report contracts are
unchanged. No database migration is required.

## Security and operations

Plan and binding inputs are bounded and structurally plus semantically
validated before database access. Attempt IDs cannot select mutable data:
attempt records are immutable, and every record is checked against the planned
job. The evaluator performs no process execution, checkout, artifact download,
or external network call.

CI should generate bindings from its successful planned jobs, ingest attempts
in a trusted step, then evaluate the complete plan. Database credentials remain
outside test-execution jobs.

## Validation

- Contract fixtures cover heterogeneous bindings, full-only miss evidence, and
  invalid aggregate recall.
- Go tests cover aggregate counts, duration, recall, deterministic group order,
  full-only misses, exact group coverage, reused attempt IDs, and plan/job
  mismatches.
- CLI tests exercise plan decoding, canonical plan identity, stored attempt
  loading, and versioned report output.
- The reference GitHub Actions flow constructs explicit bindings and produces
  one complete plan report after trusted ingestion.
