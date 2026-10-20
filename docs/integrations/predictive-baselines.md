# Compare predictive baselines

## TL;DR

- Prepare a [chronological dataset index](predictive-replay.md) and retain all indexed observation files.
- Run `predictive-baselines -index index.json -budget-ms 60000 -seed 42`.
- Compare the five policies' selected test IDs, actual cost, missed failures, and time to first failure on each holdout episode.

The command validates the complete index and observation digests before emitting a versioned JSON report. It writes no partial report if any input is missing or inconsistent. The seed is a required unsigned decimal integer. The estimated stage budget is greater than zero and at most 86,400,000 milliseconds. Up to 100,000 tests across the archive and 10,000 per episode are accepted; output is capped at 16 MiB.

Run-all is the unbudgeted full-suite reference. Explicit-impact uses only the archived `RUN_REQUIRED` decision-time candidates. Recent-failure uses the latest failure position in the replay history. Duration uses the latest historical duration. Seeded-random uses a stable hash of the seed and test identity. Four policies use the same estimated-duration budget and a 1,000 ms cold-start estimate. They skip an over-budget candidate and continue to the next. They never use held-out duration to decide whether a test fits.

The report includes `selectedTestIds` in execution order; each is a stable SHA-256 identity resolvable against the archived observation. `fullFailureCount` is the authoritative control count. `missedFailureCount` is the difference from `selectedFailureCount`. `timeToFirstFailureMs` sums actual selected durations up to the first failed test, or is null if none was selected. Actual selected time can exceed the estimated budget. Changing held-out outcomes or durations can change metrics but cannot change selected identities.

This comparison establishes reference evidence for later ranking, calibration, and safe scheduling. It does not promote a selector or set a repository risk threshold. The dataset index still cannot independently prove an undeclared group was not omitted; audit it against the retained source plan.
