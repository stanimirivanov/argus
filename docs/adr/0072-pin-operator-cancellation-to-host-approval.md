# ADR-0072: Pin operator cancellation to host approval

- Status: Proposed
- Date: 2026-10-28
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Reuse the protected operator/CI approval-file trust model without granting authority from CLI inputs.
- Pin scope, exact position and frozen request; a cancellation digest grants only that exact write.
- Record local intent or inspect it; neither command calls Perfeng or proves termination.

## Decision and consequences

PR10c adds explicit `-cancel` and `-cancellation` modes to `performance-requests`.
Host-only `ARGUS_PERFORMANCE_CANCELLATION_APPROVAL_FILE` identifies canonical
`json.Marshal(cancellationApproval)` configuration. The authenticated owner/CI
process issues it outside submitted input/checkout control with read-only mounts
or deployment ACLs. Its trusted origin is the authority, not file contents or a
command-line flag. Approval issuance/audit and host isolation remain host duties.

Approval pins scope, position and the exact original request SHA256. An optional
canonical cancellation SHA256 grants that write; omitting it permits inspection
only. Inputs cannot supply scope or broaden that grant. Canonical round-trip
validation rejects ignored/duplicate fields, aliases and invalid Unicode before
storage opens. The root rechecks the exact position, frozen hash/principal and
complete wire context, then uses the existing atomic intent/cutoff API.

Inspection performs reads only and revalidates returned evidence. Output is local
diagnostic JSON with owned intent/fingerprint; `created` describes intent retention
or exact replay, never provider cancellation. Cancellation, commit errors or short
stdout leave outcome uncertain: use explicit inspection/exact replay, not automatic
retry or new identities. File removal/ACL changes revoke future invocations; no
approval is cached. Read-only profiles may use read-only database credentials.

No schema, dependency, published wire or default worker behavior changes. Revert
the command modes or remove host configuration to roll back while retaining intent.
Provider cancellation/recovery remains PR10d and reviewed reruns remain PR10e.
