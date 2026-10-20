# Train and compare the predictive ranking baseline

## TL;DR

- Retain the complete [chronological observation archive](predictive-replay.md) and its index.
- Run `predictive-ranking -index index.json -budget-ms 60000 -seed 42`.
- Inspect the frozen coefficients, per-test feature contributions, selected IDs, and held-out metrics beside the five deterministic baselines.

The command validates the same bounded archive and temporal split as `predictive-baselines`. Its seed selects the reference random baseline only; model fitting has no randomness. The budget must be greater than zero and at most 86,400,000 milliseconds. It emits one `argus.dev/predictive-ranking/v1` JSON report, or no output on invalid input. A report above 16 MiB is rejected.

Training needs at least two eligible episodes and both passed and failed controls. Each training example sees only complete control episodes whose last group finished strictly before its feature cutoff. An unfinished earlier episode cannot supply a prior failure or duration. The fitted model uses explicit impact, prior failure rate, a prior-seen indicator, and latest-control failure. Coefficients and signed per-test contributions explain the raw risk score. The value score subtracts log estimated duration in seconds so a shorter expected test can rank higher. These scores are **not calibrated probabilities**.

All held-out tests appear in value-score order with their feature values, contributions, estimated duration, and selected flag. The policy greedily fits tests under the same estimated budget as the reference baselines, continuing after an over-budget candidate. The report nests the five baseline comparisons and provides the learned policy's full-control failure, miss, cost, and time-to-first-failure metrics. Actual selected time can exceed the estimate. Changing held-out outcomes or durations can change metrics but cannot change coefficients, scores, order, or selected IDs.

This is offline evidence for later calibration and safety work. Do not use the raw score as a failure probability or as a production selection gate. The dataset index is still a declaration: audit it against the retained source manifest and control plan to establish completeness.
