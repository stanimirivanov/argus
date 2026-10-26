# ADR-0059: Lease performance dispatch with monotonic fencing

- Status: Proposed
- Date: 2026-10-26
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Keep mutable operational claims separate from immutable pending evidence.
- Use database time, scoped row locks and a monotonically increasing fence.
- No claim grants approval, dispatch eligibility or terminal acknowledgement.

## Decision and consequences

PR8b1 adds a scoped claim sidecar owned by PerformanceStore, using the existing
bounded runtime pool. ClaimRequest locks/skips pending rows and atomically acquires
one available child. Each acquisition uses a fresh UUID token and increments its
fence. Leases accept whole microseconds from one second to five minutes. The
database clock determines lease expiry and retry availability; a lease can expire
before its caller receives it. Lost commit responses require waiting for expiry,
not sending with assumed ownership or manufacturing fresh request identities.

ReleaseRequest checks independently authorized scope, position, token, fence and
live database expiry before releasing with a zero-to-one-hour retry delay. A lost
release response is uncertain; replay may return conflict. Old, expired or released
owners cannot release again. Database guards reject deletion/truncation, scope/key
changes, fence reset and premature takeover. Expired request evidence is claimable
for readback recovery only. PR8b2 adds retained provider outcomes; PR8c adds worker
authorization, eligibility, network dispatch and recovery outside transactions.

Row locking requires UPDATE privilege on pending rows, whose existing triggers
still reject actual mutation. Future worker roles also need SELECT/INSERT/UPDATE
on claims; grant neither DDL nor deletion/truncation rights. No role grants or worker
activation are introduced here. A broker or workflow runtime is unnecessary for
this bounded capability. Fences are retained rather than deleted/reinitialized.

The additive migration creates a scoped unique index and an empty sidecar, without
backfill or heap rewrite. Pending rows take an ACCESS EXCLUSIVE lock until commit;
index creation scans all rows. Budget another scope-position B-tree plus build
workspace and WAL/replica headroom; production sizes are deployment inputs. Schedule
large builds while writers are idle. DDL and ledger updates roll back together.
Apply before new callers; existing writer/read paths remain compatible. Use the
current migrator. Rollback disables callers and preserves tables/fences. Verify empty
and released upgrades, concurrency, expiry, rollback and reopen with `make validate`
and `make db-validate` on disposable PostgreSQL 17.11.
