# ADR-0068: Append and read scoped lifecycle evidence

- Status: Proposed
- Date: 2026-10-27
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Recompare verified admission with authoritative scoped settlement/frozen work.
- Seed recovery anchors and append atomically; exact retries remain read-only.
- Revalidate canonical bytes through a pure adapter port before returning history.

## Decision and consequences

PR9b2 adds transactional append and read-only repeatable-read history/projection.
Callers authenticate scope and provide a bounded, pure `LifecycleVerifier`, usually
a closure over `perfeng.BindLifecycleAdmission` with independently trusted frozen
bytes/provider. It performs no I/O. Both admission and every observation are
revalidated; stored metadata cannot replace wire/run/context truth. Contradictory
or corrupt rows return unavailable without partial history. Unknown/wrong scope
returns not found; known admission without history reads empty, with no projection.

Appends lock pending work, validate complete bounded history and recognize exact
event replay before capacity checks. Recovery anchors and new evidence commit
together. Event advisory keys are acquired in sorted order before anchor-first
insertion, using the existing trigger namespace. Lost commit acknowledgement
requires exact retry or scoped readback, never redispatch or new identities.
Cancellation/deadlines propagate; conflicting evidence is distinct from redacted
unavailability. Late evidence remains valid and COMPLETED never means PASS.

No migration, grant, wire or runtime activation change occurs. Revert application
code while retaining append-only evidence/schema. Existing provider polling remains
admission-only; PR9c owns lifecycle consumption and PR9d owns analysis verdicts.
