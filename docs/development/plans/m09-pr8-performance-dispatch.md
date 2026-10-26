# M09-PR8 performance dispatch delivery

## TL;DR

- Deliver the dispatch outcome in dependent review-sized slices; stop after each PR.
- The HTTP transport alone neither activates dispatch nor provides durable recovery.
- Preserve frozen identities and payloads across uncertainty; keep network I/O outside transactions.

| Slice | Observable scope | Depends on | Gate |
|:--|:--|:--|:--|
| PR8a | Authenticated bounded submission and scoped provider lookup with exact context validation | PR7d2 | HTTP replay, rejection, correlation, redirect/cancellation tests, `make validate` |
| PR8b | Durable dispatch claims, fencing and retained outcomes separate from immutable pending evidence | PR8a | Live concurrent claim, expiry/fencing, rollback/reopen tests, `make validate`, `make db-validate` |
| PR8c | Application dispatch and uncertain-admission recovery with opt-in worker composition | PR8b | Crash/replay, lost-response/readback, deadline and provider-failure tests, `make validate`, `make db-validate` |

Split a later slice further if its implementation exceeds the agreed size band.
PR8c must consume both the transport and durable claim boundaries; PR8a exposes no
automatic execution path. PR9 owns lifecycle ordering and decision evidence; PR10
owns cancellation/reruns. Transport errors, including lookup absence, cannot prove
that an uncertain submission never committed. No new identities or implicit retries
may be invented to recover it. The worker must independently authorize retained
scope/endpoint/credentials, enforce deadlines and persist fenced progress.
