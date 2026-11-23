# ADR-0078: Seal rerun lineage with successor assembly

- Status: Proposed
- Date: 2026-10-29
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Immutable scoped links enforce one parent/successor and increasing positions.
- Link only freshly assembling successor batches to retained terminal evidence.
- No backfill or activation; atomic APIs and operator flow remain PR10e3/PR10e4.

## Decision and consequences

Migration 22 adds `performance_rerun_links` with both scoped pending references,
intent/request/admission/terminal pins and the Rerun v1 fingerprint. Native guards
require READ COMMITTED, fresh review/identities, matching latest terminal history,
same-transaction successor assembly and complete child coverage at commit. Terminal
recovery anchors must be seeded before linking. Update/delete/truncate are forbidden.
Unique parent/successor positions prohibit branching/merging; increasing positions
prohibit cycles. Guard locks parent then successor, sharing lifecycle's parent lock.
Batch callers must lock all parents in position order before assembling successors.

Apply before rerun APIs. The empty sidecar needs brief FK locks on pending/intent
tables, no scans/backfill/rewrite of existing evidence. Pause writers for migration;
retain schema/data on rollback and restore with their referenced evidence together.
Existing independent creation stays compatible. Runtime credentials are trusted
application access, never migration-owner/superuser authority. SQL checks relational
pins; Go/wire fingerprint authentication and approval revalidation remain required
in PR10e3/PR10e4. No provider calls, new grants, wire changes or commands activate here.
