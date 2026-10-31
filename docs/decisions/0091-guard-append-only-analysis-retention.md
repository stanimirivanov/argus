# ADR-0091: Guard append-only analysis retention

- Status: Proposed
- Date: 2026-10-30
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Expand storage with immutable scoped original analysis evidence and exact digests.
- Share pending-row/event locks with existing admission and lifecycle writers.
- Keep application append/read APIs and activation in later slices.

## Decision and consequences

PR9d3b1 adds a transactional table keyed by position and hashed provider event ID,
with provider-wide event uniqueness. Scope foreign keys and an insertion guard
require the authoritative non-rejected admission fingerprint and frozen request
SHA. Run identity agrees with retained analysis/lifecycle evidence in both insertion
orders. Serialize pending-row then existing advisory event keys; reject event reuse
across analysis/admission/lifecycle families in either order. Distinct publication
events remain retainable for later conflict projection, capped at 1000 per request.
Exact replay reconciliation before capacity checks belongs to PR9d3b2. Guards
require READ COMMITTED (the existing runtime writer isolation) and reject snapshot
writers that could miss commits while waiting on event locks.

Preserve bounded original publication/review/report bytes and verify their SHA-256
in CHECK constraints. The versioned inward fingerprint is retained, but full wire,
fingerprint and native review conformance remain adapter/application work. Direct
SQL JSON is not authenticated merely because it passes structural constraints.
Store microsecond-truncated publication TIMESTAMPTZ plus a 0..999 nanosecond remainder;
retain original bytes and keep database recording time distinct. No overall verdict
or lifecycle change follows from retaining analysis. Update/delete/truncate fail.

The migration has no backfill or rewrite. Trigger installation briefly locks the
existing settlement/lifecycle tables; schedule it with ordinary migration rollout.
Existing application error mapping treats new guard conflicts consistently. Runtime
roles need SELECT on the new table for existing trigger paths, and INSERT for later
analysis writers. Configure default privileges or pause writers and reapply existing
runtime grants after migration before resuming them. No grants are broadened by SQL.
Deploy schema before PR9d3b2 writers; application rollback keeps unused evidence.
Runtime activation and any retention/export/deletion policy remain separate work.
