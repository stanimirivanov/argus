# ADR-0077: Bind reruns to reviewed terminal predecessors

- Status: Proposed
- Date: 2026-10-28
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Reruns require retained COMPLETED, FAILED or CANCELLED provider evidence.
- Each parent may have one direct successor; further reruns form a chain.
- PR10e1 adds immutable evidence only; storage and operator activation follow.

## Decision and consequences

The owner confirmed terminal-only eligibility and single-successor chains.
Rejected, locally cut-off, active, CANCELLING and unresolved requests are not
eligible. Expiry or cancellation intent never proves termination. Previously
retained terminal recovery anchors qualify without invented intermediate history.

`requestintent.NewRerun` binds a retained parent child/intent and validated complete
history/admission to one child in a freshly compiled successor intent. Scope stays
the same; the review fingerprint and all successor request/attempt/event identities
must be fresh across parent siblings and identity roles. The versioned projection
pins both intents, parent position/request/admission, full projected terminal
metadata/evidence and successor ordinal/request. Existing fingerprints stay unchanged.

This pure inward value neither authenticates approvals nor proves that rows exist
or a supplied history is complete. Future atomic storage must reread/revalidate
authoritative admission/history and enforce immutable single-successor links.
The operator flow must freshly compile current source/catalog/policy and pin parent
evidence plus successor review in independent protected owner/CI approval. No
automatic retries, identity generation, provider I/O or activation occur here.
PR10e2 owns schema/guards, PR10e3 atomic persistence/reads and PR10e4 the operator flow.
No migration, dependency or wire changes; reverting this foundation preserves evidence.
