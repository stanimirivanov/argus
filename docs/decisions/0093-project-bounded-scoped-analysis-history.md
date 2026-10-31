# ADR-0093: Project bounded scoped analysis history

- Status: Proposed
- Date: 2026-10-31
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Traverse one scoped snapshot, holding one raw bundle at a time.
- Preserve independent per-rule outcomes and every publication's evidence pins.
- Flag distinct report identities; select no winner or aggregate verdict.

## Decision and consequences

PR9d3c scans at most 1000 event IDs and reuses indexed exact-row reads within the
same read-only repeatable-read snapshot and existing operation deadline. It keeps
only compact projected decisions, not all multi-megabyte original bundles. Pure
bounded verification/projector callbacks perform no I/O or publication of partial
results. Scope/admission, original bytes and projected event/digest/time/late pins
must agree. Commit or traversal failure returns zero projection, including when a
later row is corrupt. Concurrent appends become visible only in the next snapshot.

The inward collector owns statuses/reasons for quality, SLO and regression, plus
rule selectors, event/report identity and full provider time. Preserve late evidence
and missing/NOT_EVALUATED states; expose no overall PASS, blocking or approval.
Sort by provider publication time then event ID. Exact duplicates collapse; changed
reuse fails. Distinct report-byte digests conservatively flag conflict, including
metadata differences, while preserving all publications without newest-wins.
Measurements/effects remain accessible via the pinned original event/report.

Bound serialized compact output to 8 MiB in addition to the 1000-event cap. Exceeding
either bound fails rather than truncating decisions or fabricating completeness.
Unknown scope is not found; a known empty history is an empty projection. Collector
state is single-caller; returned results own their mutable buffers. No migration,
dependency, network consumption, activation or lifecycle transition is added.
Application rollback leaves immutable evidence untouched; PR9d3d/e own consumption.
