# ADR-0058: Retain performance intent and pending dispatch atomically

- Status: Proposed
- Date: 2026-10-25
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Persist immutable reviewed intent and all child work in one transaction.
- Exact replay is read-only; current catalog and deadlines gate new creation.
- Scope every read; keep processing claims and network calls outside this slice.

PR7c1 delivers the evidence value and schema guards. PR7c2 delivers transactional
creation, exact replay and scoped reads through PerformanceStore; PR7d adds operator
use. The adapter exposes redacted conflict/not-found/expired/unavailable errors and
preserves context cancellation; uncertain commit responses require exact replay.

## Decision and consequences

The requestintent application value owns a versioned fingerprint of the authorized
provider/principal scope, canonical review digest, catalog fingerprint, retained
policy/selection/review bytes, and ordered children with payloads, digests, identities,
timestamps and reasons. It validates inward integrity; selectionio owns wire and
approval validation. PostgreSQL never imports transport DTOs or the driving adapter.

Use the existing PerformanceStore and bounded pool. Serialize catalog publication
and request creation with the same scope lock. Insert intent and pending dispatch
rows together, enforce scoped foreign keys and identity uniqueness, and recheck
completeness, current catalog and freshness at commit. Seal child insertion after
assembly; forbid evidence updates/deletes. Exact replay compares the complete
fingerprint before freshness checks and cannot create more work or overwrite history.

Pending reads use bounded database-local keyset cursors, include expired work for
recovery, and grant no processing lease. Concurrent writes require restarting a scan
from zero; pages are not a durable snapshot. PR8 owns claims/fencing and dispatch
progress separately from immutable payload evidence. This avoids a broker or generic
job framework and keeps network calls outside transactions.

The opt-in performance-requests command first exposes read-only inspection (PR7d1).
Its local diagnostic output retains the inward evidence projection and base64
binary fields, following catalog CLI diagnostics without publishing a portable
data contract. New independently consumed contracts follow ADR-0003. Scope
configuration and
database authorization govern access. PR7d2 enables an explicit creation option;
inspection supplies no approval, processing claim or execution authority.

By owner decision, creation reads a protected authenticated operator/CI approval
file from ARGUS_PERFORMANCE_REQUEST_APPROVAL_FILE. It independently pins the review
hash, reviewer, principal, request issuer, owner routes and provider settings. The
host authenticates the approval issuer and protects file origin/ACLs; the command
cannot substitute submitted claims or verify signatures. Local artifacts use bounded
canonical internal Go projections. Creation revalidates current evidence at actual
time; uncertain/expired retries recover history through scoped inspection. No
provider call, schema change, approval service or dispatch activation is introduced.

The migration is additive, transactional and has no backfill. Apply it before a new
writer; old binaries continue to work. Rollback disables creation and retains the
tables and pending evidence. Follow existing migration-role access controls; no new
network authorization surface or credentials are introduced. Verify empty/released
migrations, rollback, uniqueness, scope, reopen and race behavior with `make validate`
and `make db-validate` against disposable PostgreSQL 17.11.
