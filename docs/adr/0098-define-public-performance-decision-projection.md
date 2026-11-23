# ADR-0098: Define a public performance decision projection

- Status: Proposed
- Date: 2026-11-02
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Publish a closed v1 metadata contract for one admitted request.
- Reuse scoped compact analysis meanings; preserve all events and independent outcomes.
- Require authorization, consistent retained evidence and complete byte bounds at export.

## Decision and consequences

Joint-plan PR10 adds `argus.dev/performance-decision-projection/v1` using existing
Effect/JSON Schema tooling. Pin change/selection/review/intent, frozen request and
target, public Run, optional retained lifecycle and every analysis publication.
Keep quality, SLO and regression independent. Empty known history has no verdict;
unavailable/corrupt reads emit no partial or empty-success document. Distinct
report-byte identities flag conflict without choosing a winner. Lateness compares
provider publication time with the original deadline.

The public allowlist excludes protected evidence bytes, credentials, storage
locations, database positions and arbitrary extensions. Enforce a complete 8 MiB
encoded budget alongside bounded arrays/text; do not truncate historical values.
The schema constrains structure/status families, not provenance, authorization,
byte counts, cross-field equality, chronology or conflict correctness. The
[integration guide](../integrations/performance-decision-projection.md) specifies
those producer/consumer invariants and disclosure rules.

This is an additive contract foundation for authorized export (joint PR11), real
joint acceptance (PR8) and campaign reporting. It grants no new runtime, database,
network, baseline or deployment authority. Go DTOs remain consumer-owned and are
introduced with the exporter. Revert removes this family without migrating data.
