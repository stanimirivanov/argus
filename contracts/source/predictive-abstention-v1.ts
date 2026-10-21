import { Schema } from "effect";
import { LocalKey } from "./repository-descriptor-v1.js";

const Count = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0));
const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Probability = Schema.Number.check(
  Schema.isGreaterThanOrEqualTo(0),
  Schema.isLessThanOrEqualTo(1),
);

/** Offline decision-time abstention and complete-control fallback evidence. */
export const PredictiveAbstentionV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-abstention/v1"]),
  calibrationVersion: Schema.Literals(["argus.dev/predictive-calibration/v1"]),
  minimumBinExamples: Schema.Int.check(Schema.isGreaterThan(0)),
  fitEpisodeIds: Schema.Array(LocalKey).check(Schema.isMaxLength(1000)),
  calibrationEpisodeIds: Schema.Array(LocalKey).check(Schema.isMaxLength(1000)),
  episodes: Schema.Array(
    Schema.Struct({
      id: LocalKey,
      mode: Schema.Literals(["eligible", "full-control"]),
      evidenceState: Schema.Literals(["calibrated", "unavailable"]),
      candidateTestIds: Schema.Array(Digest).check(
        Schema.isMinLength(1),
        Schema.isMaxLength(10000),
      ),
      requiredTestIds: Schema.Array(Digest).check(Schema.isMaxLength(10000)),
      tests: Schema.Array(
        Schema.Struct({
          id: Digest,
          repositoryKey: Digest,
          family: Schema.Literals(["functional-api", "functional-ui"]),
          probability: Probability,
          calibrationSource: Schema.Literals(["cohort", "global"]),
          priorControls: Count,
          cohortExamples: Count,
          calibrationBin: Schema.Int.check(
            Schema.isGreaterThanOrEqualTo(0),
            Schema.isLessThanOrEqualTo(9),
          ),
          binExamples: Count,
          reasons: Schema.Array(
            Schema.Literals(["cold-start", "global-calibration", "sparse-calibration-bin"]),
          ).check(Schema.isMaxLength(3)),
        }),
      ).check(Schema.isMaxLength(10000)),
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(1000)),
}).annotate({
  identifier: "PredictiveAbstentionV1",
  title: "Argus predictive abstention evaluation v1",
});

export type PredictiveAbstentionV1 = typeof PredictiveAbstentionV1.Type;
