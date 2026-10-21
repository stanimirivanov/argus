# Evaluate predictive probability calibration

Retain the complete [chronological observation archive](predictive-replay.md) and run:

~~~sh
predictive-calibration -index index.json
~~~

The command emits one `argus.dev/predictive-calibration/v1` JSON report or no output on invalid input. It needs at least three eligible training episodes. The earliest episodes fit the ranker; later training controls fit a global probability curve and supported repository/family curves. Each curve needs at least 20 examples with at least two failures and two passes. A sparse cohort uses the global curve, marked `source: "global"`. Missing global support rejects the run. Output above 16 MiB is rejected.

Inspect `fitEpisodeIds`, `calibrationEpisodeIds`, the frozen `model`, `globalCurve`, and every cohort's support and source. Each held-out prediction includes the raw score, raw logistic probability, calibrated probability, source, and full-control failure label. `quality` reports global and per-cohort Brier scores, expected calibration error, and nonempty decile bins with observed failure rates. The global entry has empty repository and family fields. Read these metrics with their counts; a small holdout does not establish a stable production threshold.

The report is offline evidence only. It does not change selection or promotion policy. Held-out labels affect quality but never the fitted model, curves, or predicted probabilities. See [ADR-0043](../decisions/0043-calibrate-predictive-risk-by-repository-and-family.md) for the split, cohort identity, and fallback rule.
