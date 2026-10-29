# ADR-0082: Validate native analysis with independent outcomes

- Status: Proposed
- Date: 2026-10-29
- Milestone: M09 - Perfeng integration
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Reuse the published Apache-2.0 analysis/v1 schema and examples without a new dependency.
- Preserve independent quality/SLO/regression claims and exact numeric evidence.
- Decoder validity never grants authenticated correlation, blocking or execution authority.

## Decision and consequences

PR9d1 pins the native analysis artifact from the existing contracts commit/bundle,
not an invented analysis-completed envelope. The offline bounded decoder reuses
strict JSON/schema machinery, rejects contradictory published outcome semantics
and preserves provenance/reference metadata without following URIs or recomputing
statistics. Quality non-pass cannot produce decisive SLO/regression, missing
samples are not zero, and SLO PASS never overrides regression FAIL.

This shared boundary precedes authenticated artifact ingestion and decision
projection. PR9d2 must establish reviewed admitted-run/frozen-resource/policy-rule
and baseline associations, authenticated reference origin and checksum/size
verification. Native IDs are not inferred from labels. PR9d3 supplies durable
decision evidence/runtime consumption. No schema migration, provider internals,
new dependency, external status/comment, overall verdict or activation is added.
