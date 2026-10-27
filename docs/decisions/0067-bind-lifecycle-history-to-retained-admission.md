# ADR-0067: Bind lifecycle history to retained admission

- Status: Proposed
- Date: 2026-10-27
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Validate canonical admission/run correlation at the pinned Perfeng adapter.
- Retain append-only scoped lifecycle observations under the existing pending-row lock.
- Keep atomic append/projection in PR9b2 and runtime consumption in PR9c.

## Decision and consequences

PR9b1 adds an owned admission binding without changing version-one outcome
fingerprints or parsing vendor payloads in PostgreSQL. The adapter revalidates
the exact retained settlement, frozen context and run identity. Rejections cannot
bind; a lifecycle settlement supplies its exact initial snapshot. Callers still
authenticate provider/principal independently. PR9b2 must compare the binding with
stored settlement/frozen-request evidence before writing or returning a projection.

The additive table binds scope and admission through foreign keys and a trigger,
serializing writers by locking pending work before examining immutable admission
and history. A shared event advisory lock prevents cross-table identity races,
including admissions written by existing binaries. Run identity, terminal state, revisions and the
1000-observation bound are guarded. Lifecycle admission anchors must be inserted
first. Late observations remain valid. UPDATE, DELETE and TRUNCATE are rejected.
The adapter owns initial run/wire truth; SQL enforces its retained relational
invariants and never derives a run from opaque provider bytes.

No backfill, wire/dependency change or runtime activation occurs. Migrate before
using new writes; grant the runtime only SELECT/INSERT on the new table alongside
existing pending/settlement permissions. Existing binaries keep working. Migration
creates an empty table/indexes, briefly locks referenced tables for foreign keys,
and rewrites no data. Roll back application code while retaining schema/evidence;
normal ownership still controls DDL and retention remains explicit future work.
