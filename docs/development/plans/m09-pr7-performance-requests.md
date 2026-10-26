# M09-PR7 request intent delivery

## TL;DR

- Deliver the approved request-intent outcome in dependent review-sized slices.
- Shared wire validation supports compilation, dispatch and lifecycle consumers.
- Stop after each issue-linked PR for human review and merge.

| Slice | Observable scope | Depends on | Gate |
|:--|:--|:--|:--|
| PR7a | Pin shared messages and validate owned request/reply evidence offline | PR5d, shared PR1 | Fixture/negative/ownership tests, `make validate` |
| PR7b | Compile explicit owner-approved selection into immutable child requests; deduplicate identical targets and retain reasons | PR7a | Review, source/pin/freshness and deterministic compilation tests, `make validate` |
| PR7c1 | Define immutable inward request evidence and additive sealed intent/pending schema | PR7b | Inward ownership/integrity and live schema guard tests, `make validate`, `make db-validate` |
| PR7c2 | Atomically retain intent and pending work; exact replay and scoped reads | PR7c1 | Rollback, concurrent creation, conflict, ownership and reopen tests, `make validate`, `make db-validate` |
| PR7d1 | Opt-in scoped request/pending evidence inspection CLI | PR7c2 | CLI/command/architecture and read-only persistence tests, `make validate`, `make db-validate` |
| PR7d2 | Opt-in reviewed creation using current scoped stored evidence and independently authorized inputs | PR7d1 | Review/authority, CLI and atomic persistence tests, `make validate`, `make db-validate` |

The original PR7 outcome is complete only after all slices merge. PR8 owns HTTP
dispatch, claims/fencing and recovery of uncertain admission; PR9 owns lifecycle
observations; PR10 owns cancellation/reruns. No broker, general queue framework,
workflow runtime or network endpoint is introduced by this plan.

PR7c was split into two reviewable slices by owner agreement. PR7c1 is a shared
schema/evidence foundation consumed by storage/dispatch and operator composition;
it does not expose persistence methods or claim that intent creation is enabled.
PR7c2 supplies the atomic store and scoped reads on that foundation. Operator
creation is exposed only by PR7d2's explicit option; pending reads do not claim or dispatch work.
PR7d is delivered as two independently usable slices to keep operator reads and
reviewed write activation within the agreed review-size band. PR7d1 does not enable
creation; PR7d2 completes that outcome with protected operator/CI approval configuration,
chosen by the owner independently of submitted reviews. Dispatch remains PR8.

Compilation must independently revalidate source/catalog/policy and owner authority;
recommendation metadata alone cannot authorize requests. Preserve exact source,
selection/policy/catalog/deployment pins, request/attempt/event identities, deadlines
and payload digests. Never derive provider service/capability/evidence references
from local labels. Keep external calls out of persistence transactions. Additive
tables precede activation; rollback disables creation while retaining pending work.
