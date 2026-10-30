# ADR-0084: Bind publications to frozen admission

- Status: Proposed
- Date: 2026-10-30
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Bind the complete publication context/public Run to authoritative admission.
- Preserve exact owned bytes/digest and frozen request/settlement pins.
- Keep native review, policy/rule verification and artifact access separate.

## Decision and consequences

PR9d2b1 reuses the lifecycle admission binder over a caller-supplied authoritative
scoped settlement and exact frozen request. Require independently authenticated
provider/principal scope, original immutable context equality and admitted public
Run equality. A body echo cannot authenticate either input. Rejected, inconsistent
or zero settlement cannot produce an admission binding.

Bound values own exact publication bytes and their SHA-256, preserving formatting,
numeric syntax and existing frozen request/admission pins. Accessors copy bytes;
the zero value has no digest and grants nothing. Bound input size before copying,
then reuse existing strict offline codecs. No I/O, wall clock, persistence or
lifecycle/decision transition occurs.

If an accepted settlement is retained, publication cannot precede its acceptance
time. Lifecycle recovery snapshots may occur after publication and do not expose
the original admission timestamp: never invent it from their later event time.
Deadline expiry and terminal lifecycle evidence do not discard valid late analysis.

This proves admitted-request correlation only. Native resource/policy/baseline
fields remain claims until PR9d2b2 supplies independently reviewed frozen native
association, exact native policy bytes and complete rule checks. PR9d2c supplies
authenticated reference/byte retrieval and checksum/size verification. Consumers
must compare returned admission pins against authoritative scoped storage before
retaining evidence. No fetch, migration, dependency, runtime activation, blocking,
baseline/execution approval or overall PASS is added. Revert this callable binding
before consumers without changing existing stored data or runtime behavior.
