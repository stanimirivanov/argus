# ADR-0094: Consume one reviewed analysis publication

- Status: Proposed
- Date: 2026-11-01
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Consume one publication through existing authenticated and reviewed adapters.
- Use authoritative scoped admission and atomic immutable retention.
- Resolve uncertain append acknowledgement only by exact verified event readback.
- Keep runtime activation and credentials separate from this callable operation.

## Decision and consequences

PR9d3d composes one bounded observation without a scheduler, migration or new
wire contract. Host-owned scope, provider/principal, report access and expected
reviewer are independent trusted inputs. The host opens a protected review
reader; this operation does not discover files, credentials or authority.
Existing HTTP/S3 byte and time limits and the bounded review loader apply.
Readers must be regular bounded file operations or honor caller cancellation.

Read retained settlement before the authenticated publication GET. Load the
review against that admission before the report GET. Revalidate report conformance
and inward exact-byte pins before atomic append. Storage revalidates against its
stored frozen child/admission with a pure callback; no external I/O takes place
inside database transactions. Existing events are never replaced.

After an append error, allow one exact scoped event read with the attempted
admission. Only identical evidence fingerprints establish successful retention.
Changed, corrupt, absent or unavailable readback returns the append failure and
zero evidence; no latest-history substitution or provider resubmission occurs.
Cancellation returns zero evidence even if the write committed; cancellation
is not proof of rollback. Later recovery must reconcile the exact identity.

Late evidence remains eligible after terminal execution. No aggregate outcome,
lifecycle transition, execution approval or baseline authority is synthesized.
PR9d3e owns activation. Revert removes the callable operation while retaining
immutable evidence and the existing schema.

## Validation

Exercise real lookup/S3 adapters through bounded transport fixtures, reviewed
conformance and pure storage revalidation. Cover wrong scope/child, unavailable
settlement/publication, invalid reviewer/report, cancellation, replay and exact
versus changed/absent/failed recovery reads. Existing database acceptance owns
atomic append and readback; run broader database/race acceptance on CI.
