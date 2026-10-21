import { Schema } from "effect";
import { LocalKey } from "./repository-descriptor-v1.js";
import { Timestamp } from "./timestamp.js";

const Count = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0));
const PositiveCount = Schema.Int.check(Schema.isGreaterThan(0));
const Probability = Schema.Number.check(
  Schema.isGreaterThanOrEqualTo(0),
  Schema.isLessThanOrEqualTo(1),
);
const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Family = Schema.Literals(["functional-api", "functional-ui"]);
const Curve = Schema.Struct({
  examples: PositiveCount,
  failures: PositiveCount,
  intercept: Schema.Number,
  slope: Schema.Number.check(Schema.isGreaterThanOrEqualTo(0)),
});
const Bin = Schema.Struct({
  lower: Probability,
  upper: Probability,
  count: PositiveCount,
  failures: Count,
  meanProbability: Probability,
  observedRate: Probability,
});
const Quality = Schema.Struct({
  repositoryKey: Schema.String,
  family: Schema.String,
  count: PositiveCount,
  failures: Count,
  rawBrier: Probability,
  calibratedBrier: Probability,
  expectedError: Probability,
  reliabilityBins: Schema.Array(Bin).check(Schema.isMinLength(1), Schema.isMaxLength(10)),
});

/** Frozen, offline repository/family calibration and held-out reliability. */
export const PredictiveCalibrationV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-calibration/v1"]),
  holdoutStart: Timestamp,
  holdoutEnd: Timestamp,
  calibrationEpochs: PositiveCount,
  learningRate: Schema.Number,
  l2Penalty: Schema.Number,
  minimumExamples: PositiveCount,
  minimumFailures: PositiveCount,
  fitEpisodeIds: Schema.Array(LocalKey).check(Schema.isMinLength(2), Schema.isMaxLength(1000)),
  calibrationEpisodeIds: Schema.Array(LocalKey).check(
    Schema.isMinLength(1),
    Schema.isMaxLength(1000),
  ),
  model: Schema.Struct({
    trainingExamples: PositiveCount,
    trainingFailures: PositiveCount,
    epochs: PositiveCount,
    learningRate: Schema.Number,
    l2Penalty: Schema.Number,
    intercept: Schema.Number,
    coefficients: Schema.Struct({
      explicitImpact: Schema.Number,
      priorFailureRate: Schema.Number,
      priorSeen: Schema.Number,
      recentFailure: Schema.Number,
    }),
  }),
  globalCurve: Curve,
  cohorts: Schema.Array(
    Schema.Struct({
      repositoryKey: Digest,
      family: Family,
      examples: Count,
      failures: Count,
      source: Schema.Literals(["cohort", "global"]),
      curve: Schema.NullOr(Curve),
    }),
  ).check(Schema.isMaxLength(100000)),
  episodes: Schema.Array(
    Schema.Struct({
      id: LocalKey,
      predictions: Schema.Array(
        Schema.Struct({
          id: Digest,
          repositoryKey: Digest,
          family: Family,
          riskScore: Schema.Number,
          rawProbability: Probability,
          probability: Probability,
          source: Schema.Literals(["cohort", "global"]),
          failed: Schema.Boolean,
        }),
      ).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(1000)),
  quality: Schema.Array(Quality).check(Schema.isMinLength(1), Schema.isMaxLength(100001)),
}).annotate({
  identifier: "PredictiveCalibrationV1",
  title: "Argus predictive calibration evaluation v1",
});

export type PredictiveCalibrationV1 = typeof PredictiveCalibrationV1.Type;
