# ADR-0061: Settle performance dispatch under live fences

- Status: Proposed
- Date: 2026-10-26
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Retain append-only outcomes separately from frozen work and operational leases.
- Settle only live scoped ownership; exact winning-owner replay survives expiry.
- Apply the additive migration and privileges before callers; enable no worker.

## Decision and consequences

PR8b2b locks pending then claim rows, checks database time after lock acquisition,
and retains canonical provider evidence plus winning token/fence. Exact replay
compares every immutable field; changed evidence/ownership conflicts. Scope-linked
FKs, unique provider event identity, closed kinds and append-only triggers enforce
storage invariants. Event uniqueness uses a trigger-computed SHA-256 key, preserving
opaque Unicode IDs without oversized B-tree entries or trusting caller hashes.
Settled claims cannot reopen, including through older SQL paths.
Scoped reads rebuild the versioned outcome and verify its fingerprint. Pending
inspection retains all immutable work; acquisition skips settled work. Authentication
and wire validation remain caller duties. No lifecycle ordering or worker is added.

The migration creates an empty sidecar with position and issuer/event indexes; it
does no backfill, existing-row scan or heap rewrite. PostgreSQL 17.11 lock probing
confirmed ShareRowExclusive locks on pending and claims until transaction commit.
Allow a short writer pause and storage/WAL growth of up to 64 KiB evidence per child
plus metadata/indexes; production retention/capacity remain deployment inputs.
Before resuming even older claim callers, grant SELECT on outcomes; settlers also
need INSERT plus existing pending/claim lock privileges. Make no DDL/update/delete
grants on outcomes. No grants or activation are automatic. Use the current migrator
before new binaries. DDL/ledger rollback together; application rollback disables
callers and retains tables/fences/evidence. Lost settlement responses require exact
replay or scoped readback, never resubmission. Verify released-schema upgrades,
concurrency, expiry, SQL guards and rollback with `make validate`/`make db-validate`.
