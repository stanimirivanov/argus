# ADR-0071: Retain cancellation before local first acquisition

- Status: Proposed
- Date: 2026-10-28
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Persist one immutable cancellation intent per scoped pending child, separately from provider state.
- Serialize intent and first-acquisition eligibility on the pending row, including native SQL guards.
- Preserve acquired claims and lookup-only recovery; no local write proves termination.

## Decision and consequences

`RequestCancellation` locks authoritative frozen pending evidence, revalidates a
canonical `Cancellation` through a bounded pure adapter verifier, and inserts the
append-only sidecar. Exact replay is read-only; changed intent conflicts. Scope
comes from independent authorization. `GetCancellation` returns owned revalidated
evidence; unknown/wrong scope is not found and corruption returns unavailable.
Intent records a caller's request even if provider disposition is already known;
it does not override acceptance, rejection, terminal evidence or approval.

First acquisition and cancellation serialize on the pending row. If intent wins,
claim selection skips unclaimed work and a database trigger rejects first-claim
inserts from legacy/native callers. If acquisition wins, that claim may already
have sent: it remains uncertain, not revoked. Existing claim rows retain their
fences and remain reacquirable only at fence >1 for existing lookup-only recovery.
Rollback leaves neither intent nor cutoff; lost acknowledgement requires exact
replay or scoped readback. No external I/O occurs in transactions.

Within a provider/principal scope, original-request and cancellation delivery IDs
share their request-source namespace. Triggers acquire the same event advisory key
in both insertion directions; fixed-width SHA256 keys bind all scope/source/ID
fields with byte separators. Request creation already serializes its scope;
cancellation takes the pending-row lock before its single event key. Intent cannot
be updated, deleted or truncated. Wire parsing stays in Perfeng, outside SQL.
The write guards require READ COMMITTED, matching the application transaction
boundary. Native repeatable-read/serializable writers fail closed (SQLSTATE 25000)
so an earlier transaction snapshot cannot hide intent/event reservations after
waiting for a lock. Read-only history projections keep their existing isolation.

Apply migration 20261028113837 before the new binary; it adds no backfill and
preserves populated request/claim/outcome/history rows. Pause workers during
upgrade. Retain the sidecar/guards on rollback and disable old workers: their
selection query can repeatedly select a cut-off child, although SQL still blocks
its first acquisition. Do not delete intent to re-enable dispatch. PR10c/PR10d own
operator/provider activation, and PR10e owns fresh reviewed reruns. Neither
intent, expiry nor local cutoff establishes CANCELLED or allows another execution.
