import { Schema } from "effect";
import { LocalKey } from "./repository-descriptor-v1.js";
import { Timestamp } from "./timestamp.js";

const Count = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0));
const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Duration = Count.check(Schema.isLessThanOrEqualTo(864_000_000_000));
const Comparison = Schema.Struct({
  name: Schema.Literals([
    "run-all",
    "explicit-impact",
    "recent-failure",
    "duration",
    "seeded-random",
  ]),
  selectedTestIds: Schema.Array(Digest).check(Schema.isMaxLength(10000)),
  estimatedSelectedDurationMs: Duration,
  actualSelectedDurationMs: Duration,
  fullDurationMs: Duration,
  fullTestCount: Count,
  fullFailureCount: Count,
  selectedFailureCount: Count,
  missedFailureCount: Count,
  timeToFirstFailureMs: Schema.NullOr(Duration),
});

/** Deterministic metrics on held-out controls; selection never reads their labels. */
export const PredictiveBaselinesV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-baselines/v1"]),
  budgetMs: Schema.Int.check(Schema.isGreaterThan(0), Schema.isLessThanOrEqualTo(86_400_000)),
  seed: Schema.String.check(Schema.isPattern(/^[0-9]+$/u)),
  holdoutStart: Timestamp,
  holdoutEnd: Timestamp,
  episodes: Schema.Array(
    Schema.Struct({
      id: LocalKey,
      comparisons: Schema.Array(Comparison).check(Schema.isMinLength(5), Schema.isMaxLength(5)),
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(1000)),
}).annotate({
  identifier: "PredictiveBaselinesV1",
  title: "Argus predictive baseline comparison v1",
});

export type PredictiveBaselinesV1 = typeof PredictiveBaselinesV1.Type;
