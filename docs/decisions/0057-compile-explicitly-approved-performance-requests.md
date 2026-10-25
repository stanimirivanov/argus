# ADR-0057: Compile explicitly approved performance requests

- Status: Proposed
- Date: 2026-10-25
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Reselect current inspected evidence; bind complete owner approval independently.
- Freeze validated provider payloads and retain every selection reason.
- Persist and dispatch only in the later agreed slices.

## Decision

Extend the existing selectionio driving adapter to assemble reviewed shared wire
evidence. Domain Select owns recommendation policy; the Perfeng contract adapter
owns interchange validation. Compilation performs no I/O or identity generation.
An authenticated caller supplies the approved canonical review digest, reviewer,
principal, request issuer and exact authorized owner routes independently from
submitted claims. The review contains the complete selected evidence and explicit
provider messages; local capability labels never become provider references.

Reevaluate current source/catalog/policy, require complete candidate coverage and
bound approval and request deadlines by selection validity. Combine identical
requests while retaining all reasons; conflicting identities or target payloads
fail atomically. Freeze exact payload bytes/digests and owned policy/review/selection
evidence so persistence and later dispatch cannot silently reconstruct requests.

## Alternatives and consequences

Trusting recommendation metadata or self-submitted approval would grant execution
without owner authority. Generating identities during compilation would destroy
replay stability. Importing transport DTOs into the performance domain would invert
its boundary. A public review-file schema and authentication implementation belong
to the later entry point; no such format is published here. Provider capability and
evidence associations require explicit owner review and independent provider admission.

This additive internal API changes no existing command, database or deployment.
Run review/freshness/pin/deduplication/ownership tests and `make validate`.
