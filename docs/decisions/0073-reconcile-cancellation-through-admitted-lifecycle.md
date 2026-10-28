# ADR-0073: Reconcile cancellation through admitted lifecycle

- Status: Proposed
- Date: 2026-10-28
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Validate retained intent, authoritative admission/run and history before provider effects.
- GET before at most one cancellation POST; recovery never submits an original request.
- Retain canonical observations atomically, preserving exact-acknowledgement and ordering guards.

## Decision and consequences

PR10d1 adds `IntegrationHTTP.ConsumeCancellation` using existing ports. Independently
configured scope/request issuer and immutable frozen work are prerequisites. Validate
canonical intent, authoritative admission/run and history before effects. Missing or
rejected admission stays with fenced recovery; local cutoff is not provider CANCELLED.

Terminal history skips network; recovery anchors seed atomically. Otherwise GET
first: current/newer retained CANCELLING or terminal evidence needs no POST.
When needed, send retained intent once; validate both responses and history by
run/context/revision, deduplicating exact snapshots before bounded projection.
Completion/failure may win. Append only the chosen observation; interim GET is
not retained when POST runs. Later GET reconciles remote failure. Lost local append
acknowledgement needs exact event readback, never a newer revision alone. Returned
observations may be stale and prove no particular delivery receipt.

Extend ADR-0070 RetryAfter hints to GET 429/503 without changing status/error text
or original submission behavior. Existing bounds apply; no sleeps/retries or network in transactions.
No schema/dependency/wire/default activation changes. PR10d2 owns traversal,
backoff, pre-admission coordination and worker composition. Revert retains evidence.
Reruns remain PR10e; absence, expiry or acknowledgement permits neither new execution nor PASS.
