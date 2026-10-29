# ADR-0079: Read historical rerun pins without renewing eligibility

- Status: Proposed
- Date: 2026-10-29
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Read scoped forward/reverse links and verify their retained historical pins.
- Restore canonical Rerun v1 records without reauthorizing execution or approval.
- Atomic creation/replay remains PR10e3b; operator activation remains PR10e4.

## Decision and consequences

`RerunSuccessor` and `RerunPredecessor` return one `RerunRecord` by independently
authorized scope and parent/successor position. A single read-only SELECT checks
referenced intent/request/admission and the pinned terminal projection. Missing or
wrong scope is not found; bad bindings, bounds or fingerprints are unavailable with
zero output. No latest-history comparison can replace the captured terminal proof.

`RestoreRerun` validates record shape and the unchanged version-one encoding; UTC
normalization preserves timestamp-equivalent fingerprints. It is an integrity
decoder, not proof of complete history, canonical provider wire authentication,
current approval or new-run eligibility. New writes must reconstruct NewRerun from
authoritative inputs inside the creation transaction and independently revalidate
owner approval. Referenced raw payload/wire validation remains at that boundary.

No schema, dependency, wire or command changes. Reads work with read-only database
profiles and retain historical evidence across later valid lifecycle observations.
Revert affects only callable readback/restoration; persisted links stay intact.
