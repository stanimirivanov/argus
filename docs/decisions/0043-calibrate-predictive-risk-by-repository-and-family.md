# ADR-0043: Calibrate predictive risk by repository and family

- Status: Proposed
- Date: 2026-10-21
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Fit the ranker on early training episodes and calibration curves on later training controls.
- Use a repository/family curve when supported, otherwise report explicit global fallback.
- Evaluate probabilities and reliability only on the untouched holdout.

## Context

ADR-0042 emits a reproducible raw ranking score. A score is useful for ordering but does not estimate failure frequency. Calibrating on the same labels used to fit the ranker would overstate reliability. Small repository/family slices can also yield unstable curves.

## Decision

`predictive-calibration` reads the complete bounded observation archive and fixed replay. A repository key is the SHA-256 digest of the test repository's provider, host, and provider repository ID. The existing stable test identity and observation contract do not change. A cohort is `(repositoryKey, family)`.

Chronologically ordered eligible training episodes are divided once: the first 70% rounded up (at least two, leaving one) fit the ADR-0042 ranker; later episodes supply calibration labels. Both classes are required for ranking. The global calibrator requires at least 20 examples, two failures, and two passes. A cohort curve has the same minimum. Unsupported cohorts explicitly use the global curve, including cohorts first seen in the holdout. If global support is insufficient, the command fails without output.

Each curve fits a nonnegative-slope logistic transformation of the frozen risk score with 120 deterministic full-batch epochs, learning rate 0.1, and L2 penalty 0.02. A nonnegative slope preserves risk ordering within a cohort. The held-out period is never used for fitting. The report includes the split IDs, ranker coefficients, curve support and source, per-test raw and calibrated probabilities, Brier scores, expected calibration error, and nonempty fixed decile reliability bins. The global quality entry has empty cohort coordinates.

## Consequences

`argus.dev/predictive-calibration/v1` is additive. Calibration does not promote a live selector, choose an abstention threshold, or claim statistical confidence from sparse holdouts. Reliability estimates describe the supplied archive and should be read with counts and bins. Source CI must still establish that the declared archive is complete and authentic. A change to cohort identity, split, fitting constants, or probability semantics requires a new report version.

## Alternatives considered

A single global curve would conceal cohort-specific behavior. Fitting separate curves for every cohort regardless of support would make tiny slices unstable. Using the held-out labels for fitting would invalidate the reliability comparison.

## Rollout and validation

This additive CLI and contract have no live rollout. Domain and CLI tests cover chronological splitting, sparse-cohort fallback, deterministic output, schema validation, and held-out label independence. Revert the command and new contract together if the report semantics need revision.
