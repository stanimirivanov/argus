# ADR-0063: Acquire performance work by authorized request issuer

- Status: Proposed
- Date: 2026-10-26
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Match both frozen request/attempt issuers before acquiring ownership.
- Preserve other issuers' first-acquisition eligibility within a shared scope.
- Keep legacy claims unfiltered and retain existing fencing/settlement guarantees.

## Decision and consequences

PR8c2a1 adds PerformanceStore.ClaimRequestForIssuer with independently authorized
provider/principal scope and request issuer. The existing bounded claim transaction
filters both identity columns before row locking/acquisition. Reject malformed
issuer/scope/lease before storage; no eligible work returns scoped ErrNotFound.
Immutable evidence, fences, expiry, release and outcome exclusion remain unchanged.

Checking the issuer only after acquisition could consume another worker's sole
first-send eligibility. Provider/principal namespaces may contain multiple reviewed
request issuers, so filtering belongs in acquisition. Legacy ClaimRequest keeps its
unfiltered behavior. The additive method needs no schema, index or privilege change;
query cost remains bounded by the runtime operation timeout and existing indexes.
No claim grants approval or authorizes submission.

PR8c2a2 retains one-shot dispatch; PR8c2b retains worker composition. This slice
activates neither. Rollback disables new callers while preserving leases/evidence.
Verify isolation, mixed issuers, concurrent acquisition, fencing and compatibility
using focused tests, `make validate` and `make db-validate`.
