# M09-PR9 performance lifecycle delivery

## TL;DR

- Deliver ordered lifecycle and decision evidence in review-sized dependent slices.
- Reuse the pinned public contract and existing admission evidence without raw artifacts.
- Keep cancellation/reruns in PR10 and general operational recovery in M10.

| Slice | Observable scope | Depends on | Gate |
|:--|:--|:--|:--|
| PR9a | Owned lifecycle observations, pinned conversion and deterministic bounded history projection | PR8c2b | Published exchange, ordering, conflict, terminal and late-evidence tests; `make validate` |
| PR9b1 | Owned admission binding and append-only lifecycle schema guards | PR9a | Pinned admission conformance, concurrent schema guards, capacity, upgrade and immutability; `make validate`, `make db-validate` |
| PR9b2 | Atomic append and scoped history/projection reads, preserving authoritative admission/run binding | PR9b1 | Exact replay, rollback, corruption, concurrent append, reopen and scoped projection; `make validate`, `make db-validate` |
| PR9c | Authenticated bounded runtime consumption into durable history, including shutdown and uncertain persistence | PR9b2 | HTTP/storage composition, provider failure, lost acknowledgement and late delivery; `make validate`, `make db-validate` |
| PR9d | Consume versioned analysis verdict references as decision evidence, preserving quality/SLO/regression/uncertainty distinctions | PR9b2 | Reviewed contract conformance, correlation, missing/invalid evidence and decision tests; `make validate` |

PR9a is a shared foundation for both durable lifecycle consumption and decision
evidence. Its callable projection does not activate polling, receive events or
replace immutable admission settlement. Split later slices further if needed;
analysis requires its published compatible contract before consumption, never
invented fields or a COMPLETED-to-PASS shortcut. The existing worker settles
admission only; settled requests are not implicitly polled for lifecycle updates.

All observations require independently authenticated provider/principal scope and
the entire original immutable context. Provider revisions order snapshots, not
arrival time. Stale delivery cannot regress state, but contradictory retained
evidence must remain visible as a conflict. Terminal state and run identity are
immutable. Late results remain valid; cancellation intent and elapsed deadlines
cannot fabricate termination. See [ADR-0066](../../decisions/0066-project-performance-lifecycle-by-provider-revision.md).

PR9a has no migration or activation; a revert removes the callable projection.
Later storage/runtime slices must define migration, concurrent admission binding,
retention, authorization and rollback before enabling consumption. Existing
cross-platform CI owns broad verification on the busy contributor host; report
focused local results separately from pending CI and do not weaken its gates.
