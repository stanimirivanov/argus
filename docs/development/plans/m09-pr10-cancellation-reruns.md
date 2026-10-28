# M09-PR10 cancellation and explicit rerun delivery

## TL;DR

- Deliver cancellation and reruns in dependent review-sized slices; stop after each PR.
- Preserve exact frozen context across uncertainty; new execution requires a new reviewed request and attempt.
- Cancellation intent, acknowledgement and deadline expiry never prove termination.

| Slice | Observable scope | Depends on | Gate |
|:--|:--|:--|:--|
| PR10a | Authenticated bounded cancellation transport with status/state and immutable context correlation | PR8a, PR9a | Pinned cancellation exchange, late delivery, terminal races, scope, redirect, backoff and cancellation tests; `make validate` |
| PR10b | Durable scoped cancellation intent with exact replay and atomic local first-send cutoff | PR10a, PR8b2b | Live concurrent cancellation/acquisition, rollback, immutable intent and reopen tests; `make validate`, `make db-validate` |
| PR10c | Explicit operator cancellation and scoped intent inspection using independently authenticated operator authority | PR10b | Authorization, invalid input before effects, exact replay and uncertain commit/output tests; `make validate`, `make db-validate` |
| PR10d | Opt-in bounded cancellation dispatch/reconciliation into lifecycle history | PR10c, PR9c | HTTP/storage composition, lost acknowledgement, cancellation/completion races, pre-acceptance absence, shutdown and restart tests; `make validate`, `make db-validate` |
| PR10e | Explicit reviewed reruns with durable parent lineage and fresh request/attempt/event identities | PR10b, PR7d2, PR9b2 | Fresh approval/source/catalog/policy, identity reuse/conflict, unresolved predecessor, lineage, replay and rollback tests; `make validate`, `make db-validate` |

Split later slices if they exceed the agreed size band. PR10a supplies shared
operator/recovery transport without activation. PR10b must serialize intent against
first-send acquisition; in-flight POSTs remain uncertain until reconciliation.
Keep network access outside transactions and intent distinct from provider state.

PR10d preserves provider/run binding, original admission and ordered lifecycle
evidence. Absence, timeout and expiry cannot authorize new execution. Redelivery
preserves event/context; lost acknowledgement requires readback/reconciliation.
The provider owns overdue-run cancellation; Argus cannot infer termination.

PR10e requires fresh compilation/approval without mutating prior requests or late
evidence. Review terminal eligibility/overlap and parent lineage policy before
implementation; this does not block transport. Analysis verdicts remain PR9d;
general scheduling, retention and operational recovery remain M10.
