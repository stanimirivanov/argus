# M09-PR8 performance dispatch delivery

## TL;DR

- Deliver the dispatch outcome in dependent review-sized slices; stop after each PR.
- The HTTP transport alone neither activates dispatch nor provides durable recovery.
- Preserve frozen identities and payloads across uncertainty; keep network I/O outside transactions.

| Slice | Observable scope | Depends on | Gate |
|:--|:--|:--|:--|
| PR8a | Authenticated bounded submission and scoped provider lookup with exact context validation | PR7d2 | HTTP replay, rejection, correlation, redirect/cancellation tests, `make validate` |
| PR8b1 | Durable scoped leases, monotonic fencing and retry release separate from immutable pending evidence | PR8a | Live concurrent claim, expiry/fencing, rollback/reopen tests, `make validate`, `make db-validate` |
| PR8b2a | Shared owned outcome values and exact frozen-context Perfeng conversion, consumed by storage and worker slices | PR8b1 | Outcome bounds, ownership, correlation and wire validation tests, `make validate` |
| PR8b2b | Retain provider outcomes atomically under live fenced ownership; completed work is no longer claimable | PR8b2a | Outcome integrity, stale acknowledgement, replay and concurrent settlement tests, `make validate`, `make db-validate` |
| PR8c1 | Lookup-only application recovery and configured Perfeng bridge over existing fenced settlement | PR8b2b | Lost-response/readback, frozen correlation, cancellation, absence and provider-failure tests, `make validate` |
| PR8c2a1 | Acquire work by independently authorized request/attempt issuer before consuming eligibility | PR8c1 | Issuer isolation, concurrent acquisition, fencing and legacy compatibility, `make validate`, `make db-validate` |
| PR8c2a2 | One-shot first-acquisition dispatch with trusted eligibility and authenticated owned outcomes | PR8c2a1 | Crash/replay, deadlines, cancellation, provider failures and durable dispatch, `make validate`, `make db-validate` |
| PR8c2b | Opt-in worker composition and operator configuration | PR8c2a2 | Complete worker integration, shutdown/polling and configuration failures, `make validate`, `make db-validate` |

Split a later slice further if its implementation exceeds the agreed size band.
PR8c1 is a callable recovery operation, with no submission path or runtime activation.
PR8c2a2 retains initial-send/crash policy; PR8c2b retains operator composition;
recovery alone does not resolve work that was never submitted and cannot be found.
PR8c must consume both the transport and durable claim boundaries; PR8a exposes no
automatic execution path. PR9 owns lifecycle ordering and decision evidence; PR10
owns cancellation/reruns. Transport errors, including lookup absence, cannot prove
that an uncertain submission never committed. No new identities or implicit retries
may be invented to recover it. The worker must independently authorize retained
scope/endpoint/credentials, enforce deadlines and persist fenced progress.
