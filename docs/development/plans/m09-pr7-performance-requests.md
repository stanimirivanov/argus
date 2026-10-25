# M09-PR7 request intent delivery

## TL;DR

- Deliver the approved request-intent outcome in dependent review-sized slices.
- Shared wire validation supports compilation, dispatch and lifecycle consumers.
- Stop after each issue-linked PR for human review and merge.

| Slice | Observable scope | Depends on | Gate |
|:--|:--|:--|:--|
| PR7a | Pin shared messages and validate owned request/reply evidence offline | PR5d, shared PR1 | Fixture/negative/ownership tests, `make validate` |
| PR7b | Compile explicit owner-approved selection into immutable child requests; deduplicate identical targets and retain reasons | PR7a | Review, source/pin/freshness and deterministic compilation tests, `make validate` |
| PR7c | Atomically retain request intent and pending dispatch work; exact replay and scoped pending reads | PR7b | Rollback, concurrent creation, conflict, ownership and reopen tests, `make validate`, `make db-validate` |
| PR7d | Opt-in request creation/read CLI using current scoped stored evidence and reviewed inputs | PR7c | CLI/command/architecture and persistence tests, `make validate`, `make db-validate` |

The original PR7 outcome is complete only after all slices merge. PR8 owns HTTP
dispatch, claims/fencing and recovery of uncertain admission; PR9 owns lifecycle
observations; PR10 owns cancellation/reruns. No broker, general queue framework,
workflow runtime or network endpoint is introduced by this plan.

Compilation must independently revalidate source/catalog/policy and owner authority;
recommendation metadata alone cannot authorize requests. Preserve exact source,
selection/policy/catalog/deployment pins, request/attempt/event identities, deadlines
and payload digests. Never derive provider service/capability/evidence references
from local labels. Keep external calls out of persistence transactions. Additive
tables precede activation; rollback disables creation while retaining pending work.
