import { Schema } from "effect";
import { PredictiveBaselinesV1 } from "./predictive-baselines-v1.js";
import { LocalKey } from "./repository-descriptor-v1.js";

const Count = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0));
const Duration = Count.check(Schema.isLessThanOrEqualTo(864_000_000_000));
const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Factors = Schema.Struct({
  explicitImpact: Schema.Number,
  priorFailureRate: Schema.Number,
  priorSeen: Schema.Number,
  recentFailure: Schema.Number,
});

/** Frozen, uncalibrated model and full-control ranking evidence. */
export const PredictiveRankingV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-ranking/v1"]),
  model: Schema.Struct({
    trainingExamples: Count,
    trainingFailures: Count,
    epochs: Schema.Int.check(Schema.isGreaterThan(0)),
    learningRate: Schema.Number,
    l2Penalty: Schema.Number,
    intercept: Schema.Number,
    coefficients: Factors,
  }),
  baselines: PredictiveBaselinesV1,
  episodes: Schema.Array(
    Schema.Struct({
      id: LocalKey,
      rankedTests: Schema.Array(
        Schema.Struct({
          id: Digest,
          features: Factors,
          contributions: Factors,
          riskScore: Schema.Number,
          valueScore: Schema.Number,
          estimatedDurationMs: Duration,
          selected: Schema.Boolean,
        }),
      ).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
      comparison: Schema.Struct({
        selectedTestIds: Schema.Array(Digest).check(Schema.isMaxLength(10000)),
        estimatedSelectedDurationMs: Duration,
        actualSelectedDurationMs: Duration,
        fullDurationMs: Duration,
        fullTestCount: Count,
        fullFailureCount: Count,
        selectedFailureCount: Count,
        missedFailureCount: Count,
        timeToFirstFailureMs: Schema.NullOr(Duration),
      }),
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(1000)),
}).annotate({
  identifier: "PredictiveRankingV1",
  title: "Argus predictive ranking evaluation v1",
});

export type PredictiveRankingV1 = typeof PredictiveRankingV1.Type;
