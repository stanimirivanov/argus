# ADR-0065: Opt in to configured performance workers

- Status: Proposed
- Date: 2026-10-27
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Enable one independently authorized provider/principal/request-issuer binding explicitly.
- Reuse the control-plane deployment and runtime role; retain first-acquisition-only dispatch.
- Cancel worker I/O on shutdown and join it before closing database resources.

## Decision and consequences

PR8c2b composes the existing store, authenticated HTTP adapter and one-shot dispatcher
in the control plane. Default/false configuration starts no worker or provider I/O.
Enabled configuration is validated offline before resources open; immutable messages
never choose endpoints or credentials. Lease and polling durations are bounded.
The application loop runs serially immediately and once per scheduler tick, waiting
after success, idle and errors. Operational logs omit raw errors and retained data.

The worker inherits signal cancellation, separately from the existing HTTP/DBOS
drain lifetime. Application cleanup cancels and joins it before DBOS/pool cleanup;
server startup failure also joins it. Multiple processes still use database fences.
This introduces no service, schema, dependency or privilege change and does not
add a direct-database operator executable (ADR-0022).

ADR-0064 remains authoritative: reacquisitions only look up, 404 never authorizes
POST, and crash-before-send/lost acquisition acknowledgement can strand absent work.
No POST replay, automatic release or dead-letter handling is introduced. Disable the flag and restart to
roll back, retaining immutable evidence and leases. Verify polling/shutdown and
configuration tests, the authenticated database/provider flow, `make validate`
and `make db-validate`.
