# ADR-0040: Replay predictive evidence with a fixed temporal cutoff

- Status: Proposed
- Date: 2026-10-20
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- An explicit dataset index declares every observation group in each decision episode.
- A fixed holdout start separates training labels from held-out decisions; a training control must finish before that start.
- Any source pull request spanning both splits is rejected to avoid version leakage.
- The replay identifies archived observations by digest and never copies outcome labels into the split report.

## Context

ADR-0039 captures one complete group at a time. PostgreSQL attempts do not provide a historical manifest/change join or prove which groups belonged to a past decision. A replay assembled by filesystem order, recency, or label timing could include future data or omit an inconvenient control.

## Decision

`predictive-replay` accepts a versioned index with a holdout interval and named episodes. Each episode lists the exact archived observation files for one change and manifest. The command validates every digest, bounds and confines archive reads, requires group identity and feature consistency, rejects duplicate decisions, groups, attempts, and observation digests, then sorts episodes by feature cutoff and ID.

Training episodes must have a feature cutoff before the holdout start and every control completed strictly before it. An earlier decision whose control finishes later is explicitly excluded. Holdout episodes start at the cutoff and precede the exclusive holdout end. Later episodes are excluded. Every holdout history contains only the eligible training episode IDs. A source pull request cannot have eligible episodes on both sides of the split.

The versioned replay output records split, exclusion reason, counts, and observation digests. Consumers load full features and labels from the digest-bound v1 observation envelopes. No training, scoring, or live selection happens in this slice.

## Consequences

The index is a declaration by the archive producer, not independent proof that no group was left undeclared. Source CI must retain the original manifest and control plan to attest completeness. Missing *declared* groups fail closed. Replay does not backfill or infer historical evidence. A fixed cutoff gives a reproducible holdout, while rolling-origin and model-specific evaluation can be added later without changing these v1 meanings.

## Compatibility and migration

`argus.dev/predictive-dataset-index/v1` and `argus.dev/predictive-replay/v1` are additive offline contracts. Existing observations and live execution remain valid. Changing split or digest semantics requires a new contract version.

## Validation

Focused tests cover order independence, multi-group replay, late-label exclusion, cross-split pull rejection, missing and duplicate groups, tampering, and path escape. Contract generation, architecture checks, and repository verification validate the boundary.
