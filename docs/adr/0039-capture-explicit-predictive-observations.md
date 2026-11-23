# ADR-0039: Capture explicit predictive observations for offline evaluation

- Status: Proposed
- Date: 2026-10-20
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Capture one immutable change, decision-time manifest, and complete full-suite attempt per executable group.
- Bind the archive to the exact manifest digest and control identity; never infer a control from recency.
- Archive bounded features and labels separately, without raw patches, failure messages, or artifacts.
- Runs without the original evidence are ineligible for historical replay.

## Context

Existing PostgreSQL stores immutable attempts by ID, but not complete historical manifests or a chronological manifest-to-change feed. A reconstructed join could attach a later decision or control to an earlier change. Offline ranking would then leak future information or mislabel a test.

## Decision

`predictive-evidence` accepts three explicit versioned files: change set, executable manifest, and full-suite attempt. Functional API manifest identity uses the established canonical contract digest; browser manifest identity uses the exact file bytes, matching `run-browser`. The command validates each document, repository and revision coordinates, decision/control time ordering, exact candidate-result membership within one group, and the attempt's manifest reference before writing any output.

The observation stores change paths, file kinds and patch availability, selection reasons, mappings, and result outcomes/durations. It does not store patches, failures, or artifacts. Its SHA-256 covers the canonical JSON payload, excluding the envelope digest. `featureCutoffAt` is the change observation time for functional API manifests, a conservative cutoff because those manifests do not record their own evaluation time; browser manifests use their explicit evaluation time. One observation covers one test repository/adapter group, including browser project and environment. Multi-group replay assembles these observations in M08-PR2.

Skipped and execution-error results cannot serve as complete failure labels and are rejected by this first archive version. The source CI system owns the authenticity of supplied files and artifact retention. This command is an offline verifier, not an authority to select or run tests.

## Consequences

Existing selection, execution, and database contracts do not change. Older runs are replayable only when their original immutable inputs are available. Archive producers must retain the input files and govern the observation's paths and repository identities under their own privacy and retention policy. No database backfill is inferred.

## Compatibility and migration

The new `argus.dev/predictive-observation/v1` envelope is additive. A new version is required if label meaning or canonical digest rules change. Existing binaries and stored attempts remain valid.

## Validation

Contract generation, focused API/browser capture tests, incomplete-control and mismatch rejection, architecture checks, and `make verify` prove the boundary. Temporal leakage tests and model training are later M08 slices.
