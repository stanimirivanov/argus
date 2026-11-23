# ADR-0090: Preserve inward reviewed analysis evidence

- Status: Proposed
- Date: 2026-10-30
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Keep original publication, protected review and report bytes as owned evidence.
- Bind immutable admission/request/Run, principal, event and precise provider time.
- Leave atomic storage, scoped projection and consumption to dependent slices.

## Decision and consequences

PR9d3a introduces an inward value with bounded opaque JSON and a versioned SHA-256
fingerprint over admission/request/Run, principal, publication event/time and all
three exact-byte digests. Normalize time zones to UTC without rounding nanoseconds.
Preserve original protected review bytes in the existing loader; its bundle and
file digest alone cannot reproduce whitespace for future readback verification.

The adapter converts opaque reviewed conformance only after reloading those bytes,
rechecking review/report associations and comparing every original digest. Inward
construction checks bounds/ownership, not vendor schemas or authentication. Storage
must compare independently authorized scope, frozen request and authoritative
settlement; possessing a fingerprint does not grant approval or admission authority.

Independent quality/SLO/regression remain in unchanged report bytes. Publication
arrival, execution completion and deadline expiry never become overall PASS or
execution/baseline promotion. Late valid evidence remains usable. This foundation
is shared by later atomic retention and scoped projection. There is no migration,
network operation, dependency or worker activation; reverting requires no data work.
