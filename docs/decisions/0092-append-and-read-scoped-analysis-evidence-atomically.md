# ADR-0092: Append and read scoped analysis evidence atomically

- Status: Proposed
- Date: 2026-10-31
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Compare scope and authoritative stored request/admission before accepting evidence.
- Reconcile exact event retries before insertion and capacity checks.
- Read one bounded original bundle; broader traversal/projection follows in PR9d3c.

## Decision and consequences

PR9d3b2 adds atomic append under the existing pending-row lock and scoped exact-event
readback under a read-only repeatable-read snapshot. A pure bounded context-aware
verifier rechecks original bytes against the stored frozen child, independently
configured reviewer and retained admission. Compare complete fingerprints and the
recovery anchor. Existing database guards arbitrate provider-wide event/run identity.
No network or protected-file lookup occurs inside transactions.

Exact retry is read-only even at the 1000-event capacity. Changed event reuse is a
conflict; corrupt retained bytes/projections fail unavailable without partial output.
Lost commit acknowledgement requires exact retry/readback, never provider resend.
Wrong/unknown scope/event is not found; known mismatched admission conflicts. Restore
publication nanoseconds from the existing timestamp/remainder and preserve all bytes.

Exact reads bound each response to one bundle instead of loading potentially 1000
multi-megabyte reports. History traversal and independent decision projection remain
PR9d3c. Host authorization and expected reviewer identity remain independent trust
inputs; revalidation does not renew original authentication or grant approval.
No migration, dependency, lifecycle transition or runtime activation is added.
Reverting application code leaves retained immutable evidence intact.
