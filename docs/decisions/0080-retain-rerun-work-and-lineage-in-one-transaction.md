# ADR-0080: Retain rerun work and lineage in one transaction

- Status: Proposed
- Date: 2026-10-29
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Revalidate prepared approved bindings under ordered parent locks before new work.
- Atomically seed terminal recovery anchors, successor work and complete lineage.
- Historical exact replay precedes current eligibility; provider I/O stays outside storage.

## Decision and consequences

`CreateReruns` accepts an immutable successor intent and complete `RerunCreation`
bindings with trusted pure wire verifiers. Ordinals and parents must be unique;
prepared proofs must reconstruct exactly before I/O. Callers independently approve
fresh source/catalog/policy compilation and every parent/proof pin.

The bounded transaction shares publication/creation's authority-scope advisory lock.
Exact historical replay uses the transaction's reader before new eligibility checks;
changed or unlinked legacy intent conflicts. New writes lock parents in position
order, compare retained parent fingerprints/admission, reread complete verified
history and reconstruct NewRerun. Missing anchors lock global event keys in order.
Anchor, successor and link writes commit together with existing freshness/catalog
and lineage guards. No pool re-entry or provider calls occur inside transactions.

A commit error proves no success; recover by exact replay or scoped inspection
without regenerating request/attempt/event identities. Rollback retains no partial
new work or anchors. No schema/wire/dependency change or operator activation;
PR10e4 owns the protected approval and fresh compilation workflow. Disable callers
on rollback, retaining migration 22 and immutable evidence.
