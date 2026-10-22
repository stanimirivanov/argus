# ADR-0049: Opt in to browser staged execution

- Status: Proposed
- Date: 2026-10-22
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## Context

M08 shared staged invariants and functional API execution now need a browser
consumer. Browser execution adds project variants, environment pins and mandatory
artifact verification. A portfolio's own candidate list cannot prove complete
native discovery. Widening v1 would let legacy consumers mistake partial work
for their original selected/full-suite protocol.

## Decision

Add explicit v2 browser plans, adapter requests and immutable attempts. Keep
reviewed bindings v1, which already includes project/environment dimensions, and
keep result v1 with unchanged exact evidence semantics. Default v1 commands stay
compatible; new behavior requires explicit schedule input and trusted whole-file
SHA-256. Shadow is the default opt-in mode, with relevant-now and a full independent
control; staged requires the exact later complement. The mode is independently
required CI configuration, not authorization supplied by the plan.

Verify complete independent manifest/discovery/bindings at both planner and runner
boundaries. Correlate targeted manifests with the canonical catalog evidence digest
and observation metadata, and pin independent raw discovery bytes in plans and
attempts. Cover all discovered variants, including project/revision/title, across
reviewed groups. Before each nonempty invocation, the reference adapter verifies
fresh unfiltered native project inventory against complete request candidates.
Preserve early order and use reviewed topological order for later/control work.
Reject cross-group dependencies; CI serializes groups sharing capacity and must
run mandatory later/control passes even on early failures. Empty phases create no
adapter invocation or synthetic attempt.

Reuse strict browser outcome/artifact invariants and verify file checksums before
returning evidence. Explicit version dispatch at process, ingestion and storage
boundaries supports v2 without widening legacy-only consumers. Nullable relational
provenance and a conditional version/stage/mode constraint extend the existing
immutable browser attempt table. Omit absent new payload fields to preserve v1
fingerprints; cross-check header provenance against checksummed payload on read.

## Consequences and rollout

Deploy the forward migration and updated readers/runner/adapter before opt-in.
Old writers remain valid; old readers cannot interpret v2. Disable opt-in for
behavior rollback and retain schema and evidence. Constraint validation scans
and locks the header table; no backfill or rewrite is needed. See the
[operational guide](../integrations/browser-staged.md).

Inputs remain offline historical episodes; this slice does not create live
predictions or release authority. CI input authentication, checkout provisioning,
resource coordination and genuine evidence remain external trust boundaries.
Aggregate monitoring and promotion belong to M08-PR12/13.

## Verification

Run `make ui-validate`, `make db-validate` and `make validate`. Tests exercise real
Chromium v1/v2 execution and complete inventory, project/environment substitution,
exact partitioning, missing or corrupt artifacts, cancellation, empty phases,
strict v1 compatibility, released-schema upgrades, concurrent retries, conflicts
and database provenance constraints.
