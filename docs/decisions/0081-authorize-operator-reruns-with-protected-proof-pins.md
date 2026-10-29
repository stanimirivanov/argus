# ADR-0081: Authorize operator reruns with protected proof pins

- Status: Proposed
- Date: 2026-10-29
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- The authenticated operator/CI host pins new review and complete parent proofs.
- Rerun creation freshly compiles current evidence, verifies parents and writes atomically.
- Missing write fingerprints grant scoped inspection only; uncertainty uses readback.

## Decision and consequences

`ARGUS_PERFORMANCE_RERUN_APPROVAL_FILE` is canonical `json.Marshal(rerunApproval)`:
existing RequestApproval plus unique parent positions/review digests, zero-based
successor ordinals and optional Rerun fingerprints. The host authenticates issuance
and protects file origin/ACLs outside submitted checkout/input control; bytes are
not authentication. Submitted flags cannot supply scope or approval.

`-rerun` requires every fingerprint and independently pins the canonical new review
before storage opens. Existing current source/catalog/policy compilation runs with
the actual clock. Reconstruct scoped parent intent, frozen child, admission and
wire-verified history, then compare NewRerun to its protected pin before CreateReruns.
The atomic store rechecks current state; neither snapshot nor restoration grants a send.

`-lineage` inspects one approved parent/review, optionally requiring the exact proof.
Omitting a proof grants inspection only. Its port exposes reads alone and supports
read-only database profiles. Output is metadata, not raw provider bodies. Require
exit zero/full output; failed output cannot undo creation. After uncertain/expired
creation use inspection with original pins, never new identities or automatic retry.

No provider calls, new schema, dependency or wire change. Existing worker may
dispatch committed new work if enabled. Rollback disables these command modes while
retaining migration 22/evidence; host approval issuance remains deployment-owned.
