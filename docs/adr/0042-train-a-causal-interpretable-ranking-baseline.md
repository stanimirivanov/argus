# ADR-0042: Train a causal interpretable ranking baseline

- Status: Proposed
- Date: 2026-10-20
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Fit a small linear failure-risk model from eligible training controls only.
- Build each training example with labels from controls completed before that decision.
- Freeze coefficients before scoring holdout tests; report every feature effect and the existing five reference policies.
- Keep scores offline and uncalibrated until later M08 calibration and safety gates.

## Context

ADR-0041 compares fixed policies on a temporal holdout. A ranking model needs an equally reproducible baseline without learning from future control results. The replay's training split says a control completed before the *holdout* start; it does not by itself say the control was available before every other training decision. Treating all training labels as prior history would leak later results into earlier training features.

## Decision

`predictive-ranking` reads the same bounded, digest-verified archive and fixed replay as `predictive-baselines`. A training episode is available to another decision only after its last declared control group completes, strictly before that decision's feature cutoff. All groups in one episode become available together. Completion-time ties use episode ID for stable ordering. The fitted examples use four decision-time signals: explicit `RUN_REQUIRED` impact, prior failed/seen control ratio, whether the test was seen before, and whether its latest completed control failed. Unseen tests have zero historical signals.

Batch logistic loss fits an intercept and four coefficients in 80 deterministic full-batch epochs with learning rate 0.5 and L2 coefficient penalty 0.05. At least two training episodes and both passed and failed training controls are required. Examples are sorted by decision cutoff, episode ID, and stable test ID. The model is frozen before any holdout score is calculated. Its risk score is the intercept plus the four displayed signed contributions. For a cost-aware ranking, the value score subtracts the natural logarithm of estimated duration in seconds; a zero estimate is clamped to one millisecond for this offset. Historical duration comes from the latest completed eligible control, and unseen tests use the 1,000 ms baseline estimate. The score is an ordering device, not a calibrated failure probability.

The ranked policy walks value-score order with stable test-ID ties, selecting each test that fits the same remaining estimated-duration budget as ADR-0041. It reports selected IDs, control failure misses, actual cost, and sequential time to first failure alongside all five reference policies. Held-out outcome and actual duration are read only for those metrics, never for training, scores, order, or budget fit.

## Consequences

An archive without enough eligible labels fails rather than producing a misleading model. This first model has no repository or family calibration, uncertainty abstention, resource/dependency scheduling, or live selector authority. Sparse histories may yield unstable coefficients, which remain visible in the immutable report for later calibration and monitoring. The report is capped at 16 MiB.

## Compatibility and migration

`argus.dev/predictive-ranking/v1` and the new command are additive. The existing observation, replay, and baseline wire formats remain unchanged. Control completion is loaded from the already versioned observation envelope into the offline domain view. Changing feature definitions, fitting settings, or value-score meaning requires a new ranking contract version.

## Validation

Tests cover delayed-label exclusion, deterministic fitting and ordering, holdout-label independence, insufficient-data rejection, score decomposition, portable schema validation, and no partial output. Repository verification checks generated contracts and architecture boundaries.
