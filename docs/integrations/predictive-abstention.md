# Evaluate predictive abstention and fallback

Retain the complete [chronological observation archive](predictive-replay.md) and run:

~~~sh
predictive-abstention -index index.json
~~~

The command emits one `argus.dev/predictive-abstention/v1` JSON report or no output on invalid input. It uses the same bounded archive and chronological fit/calibration/holdout split as [probability calibration](predictive-calibration.md). Output above 16 MiB is rejected.

Each held-out test records its calibrated probability, prior completed-control count, repository/family calibration source and support, probability decile, number of later-training examples in that local decile, and zero or more reason codes. `cold-start` means no prior completed control. `global-calibration` means the local cohort curve lacked support. `sparse-calibration-bin` means fewer than three examples fell in that decile under the local curve. These are evidence gaps, not measured failure outcomes; the three-example threshold is a conservative heuristic.

An episode with any reason is `full-control`, and `requiredTestIds` lists **all** tests in stable ID order. An episode without reasons is `eligible` for later predictive scheduling and has an empty required list. `candidateTestIds` always records the complete set. When valid archived controls cannot fit calibration, `evidenceState` is `unavailable`, the episode requires full control, and no probability or per-test evidence is emitted. Malformed or incomplete archives fail without output. This report neither executes tests nor authorizes omissions. Held-out control labels and durations do not affect the safety decision. Inspect the retained source plan to establish that the archived candidate set is complete. See [ADR-0044](../decisions/0044-abstain-on-weak-predictive-evidence.md) for the policy and compatibility boundary.
