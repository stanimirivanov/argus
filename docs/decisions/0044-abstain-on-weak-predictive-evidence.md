# ADR-0044: Abstain on weak predictive evidence

- Status: Proposed
- Date: 2026-10-21
- Milestone: M08 - Predictive optimization
- Deciders: Argus maintainers
- Supersedes: None
- Superseded by: None

## TL;DR

- Require completed prior controls, a supported repository/family curve, and at least three calibration examples in the prediction's probability decile.
- If one test lacks this evidence, mark its entire held-out episode for full-control execution.
- If valid archived controls cannot fit a calibrator, require full control without inventing probabilities.
- Keep this gate offline until later scheduling and promotion work supplies deployment authority.

## Context

ADR-0043 produces calibrated failure probabilities and explicitly falls back to a global curve for sparse cohorts. A valid global probability does not establish local reliability for a particular repository and family. Likewise, a local curve can have little support near a particular probability, and a test with no completed prior control is a cold start. A scheduler must be able to see those limits before trying to save time.

## Decision

`predictive-abstention` replays the same complete archive and runs the frozen ADR-0043 calibration. It reads no held-out outcome or duration to make a safety decision. For each held-out test, it records prior completed-control count, cohort calibration support, calibrated probability, its fixed decile (0 through 9), and how many later-training calibration examples occupy that local decile.

It emits stable reasons: `cold-start` when there is no prior completed control, `global-calibration` when the repository/family has no supported local curve, and `sparse-calibration-bin` when a supported local curve has fewer than three examples in the predicted decile. Reasons can coexist. The three-example minimum is a conservative coverage heuristic, not a confidence interval or guarantee of accuracy. Missing evidence cannot be interpreted as zero risk.

If any test abstains, the whole episode is `full-control`, and `requiredTestIds` contains every held-out candidate in stable ID order. Otherwise the episode is `eligible` with an empty required list. Both states include the complete `candidateTestIds` set. If the archive is structurally valid but calibration cannot be fitted, the episode is `full-control` with `evidenceState: "unavailable"`, empty split IDs and test predictions, and identical candidate and required ID lists. Malformed or incomplete archives still fail without output. Eligibility grants no selection or omission authority in this PR; a later scheduler must still obey hard policy and budget rules. The report is capped at 16 MiB and uses additive `argus.dev/predictive-abstention/v1` JSON. Observation, replay, ranking, and calibration v1 contracts do not change.

## Alternatives considered

A fixed probability cutoff would conflate estimated failure risk with evidence reliability. Per-test fallback could silently leave neighboring tests unexecuted when episode-level evidence is weak. Using held-out reliability bins to decide the same holdout would leak future labels into the decision.

## Consequences

Conservative fallback can erase time savings, especially for new tests and small cohorts. The report makes that cost visible and preserves full-control coverage. Changing reason meanings, bin thresholds, or episode mode semantics requires a new report version. Source CI still owns archive authenticity and completeness.

## Rollout and validation

This is an additive offline command; it dispatches no tests and changes no live selector. Domain and CLI tests cover eligible, cold-start, global-curve, thin-bin, unavailable-calibration, full-control coverage, declaration-order stability, malformed archives, and held-out-label independence. Revert the command and its contract together if the policy is revised.
