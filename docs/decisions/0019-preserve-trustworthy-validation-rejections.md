# ADR-0019: Preserve trustworthy validation rejections

- Status: Accepted
- Date: 2026-09-28
- Milestone: M06 - Validated adaptation
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Keep successful validation evidence unchanged and publish policy rejection as
  a separate `validation-rejection/v1` contract.
- Record only trustworthy outcome mismatches; infrastructure, adapter, source-
  integrity, and restoration errors remain errors and are not learning labels.
- Persist one immutable rejection per deterministic validation ID with exact-
  retry and conflict semantics.
- A rejected command still exits non-zero even though it emits portable JSON.

## Context

ADR-0015 defined a three-phase proof but the implementation discarded every
completed run when the observed sequence was not `failed → passed → failed`.
That made false diagnoses, ineffective candidates, and weak negative controls
indistinguishable from an adapter that never ran. It also removed the negative
examples needed to evaluate later repair classes and policy promotion.

Successful `validation-evidence/v1` is already consumed by draft pull-request
publication. Widening it to include unsuccessful attempts would weaken its
meaning and could let a rejection reach review automation. Conversely, treating
timeouts, malformed output, source mutation, or failed restoration as a
candidate-quality label would train policy from untrustworthy execution.

## Decision

Argus introduces `argus.dev/validation-rejection/v1` as a sibling of successful
validation evidence. It retains the proposal and test identity, adapter
version, exact edit, all prepared source digests, verified restoration digest,
the completed ordered run prefix, and one policy mismatch:

- the original passed instead of reproducing failure;
- the candidate failed instead of repairing the test; or
- the negative control passed instead of discriminating behavior.

The validation command writes the rejection document to stdout and returns a
non-zero error. Existing success output and review-publication input remain
unchanged. Adapter-reported `error`, process failure, correlation failure,
adapter-version drift, source mutation, and restoration failure emit no
rejection contract.

PostgreSQL stores one immutable rejection per deterministic validation ID. The
header and completed run prefix are written in one transaction. Identical
retries return the existing record; different evidence under the same ID is a
conflict. Reads use a repeatable-read snapshot and revalidate the reconstructed
domain document.

## Alternatives considered

### Widen successful validation evidence

- Benefits: One contract and one persistence path.
- Costs and risks: Success would no longer be a reliable publication gate, and
  every consumer would need to reimplement disposition checks.
- Reason not selected: Keeping positive proof and negative learning evidence
  as distinct types makes unsafe use structurally harder.

### Persist every failed execution

- Benefits: Maximum diagnostic volume.
- Costs and risks: Infrastructure outages, malformed output, and corrupted
  workspaces would become misleading repair-quality labels.
- Reason not selected: Only correlated outcomes with verified restoration are
  trustworthy policy evidence.

### Log rejection without a contract

- Benefits: Smaller implementation.
- Costs and risks: Logs are not portable, idempotent, schema-validated, or
  suitable for chronological evaluation.
- Reason not selected: Rejection evidence is a product input, not only an
  operator diagnostic.

## Consequences

### Positive

- Argus retains false diagnoses, ineffective repairs, and weak controls as
  explicit negative examples.
- Review publication continues to accept only successful evidence.
- Exact retries and conflicts are deterministic across process restarts.

### Negative

- Validation callers must capture stdout even when the process exits non-zero.
- A new public contract, migration, tables, and command modes require support.

### Neutral or follow-up

- Successful validation persistence and aggregate learning queries remain
  separate work.
- The compatible field-rename class may reuse this evidence boundary but needs
  its own versioned impact and adaptation-policy design.

## Compatibility and migration

This is an additive sibling contract and does not change
`validation-evidence/v1`. Existing successful consumers remain compatible.
Apply migration `20260928121500_add_validation_rejections.sql` before using the
new ingestion commands. Rollback is application rollback plus leaving the
additive evidence tables in place; no existing row meaning changes.

## Security and operations

Source content and credentials are not stored. Paths, byte spans, bounded
failure messages, hashes, and test metadata remain untrusted until contract and
domain validation succeeds. The command opens PostgreSQL only after validation.
Operators should retain rejection JSON when ingestion is temporarily
unavailable and retry it by validation ID.

## Validation

- Effect Schema and Go validate a shared positive fixture.
- Service tests cover every policy rejection and verified restoration.
- Persistence tests cover round trip, exact retry, conflict, restart, and SQL
  constraints through the full migration chain.
- `make validate` and `make db-validate` exercise the complete repository.
