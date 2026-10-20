import { Schema } from "effect";
import { LocalKey } from "./repository-descriptor-v1.js";
import { Timestamp } from "./timestamp.js";

const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const EpisodeID = LocalKey;

/** Explicit membership is required because archived attempts cannot reveal missing groups. */
export const PredictiveDatasetIndexV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-dataset-index/v1"]),
  holdoutStart: Timestamp,
  holdoutEnd: Timestamp,
  episodes: Schema.Array(
    Schema.Struct({
      id: EpisodeID,
      observations: Schema.Array(
        Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(4096)),
      ).check(Schema.isMinLength(1), Schema.isMaxLength(100)),
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(1000)),
}).annotate({
  identifier: "PredictiveDatasetIndexV1",
  title: "Argus predictive dataset index v1",
});

export const PredictiveReplayV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-replay/v1"]),
  holdoutStart: Timestamp,
  holdoutEnd: Timestamp,
  trainCount: Schema.Int.check(Schema.isGreaterThanOrEqualTo(0)),
  holdoutCount: Schema.Int.check(Schema.isGreaterThanOrEqualTo(0)),
  excludedCount: Schema.Int.check(Schema.isGreaterThanOrEqualTo(0)),
  episodes: Schema.Array(
    Schema.Struct({
      id: EpisodeID,
      featureCutoffAt: Timestamp,
      observationSha256: Schema.Array(Digest).check(Schema.isMinLength(1), Schema.isMaxLength(100)),
      split: Schema.Literals(["train", "holdout", "excluded"]),
      exclusionReason: Schema.NullOr(
        Schema.Literals(["control-after-training-cutoff", "outside-holdout-window"]),
      ),
      history: Schema.Array(EpisodeID).check(Schema.isMaxLength(1000)),
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(1000)),
}).annotate({
  identifier: "PredictiveReplayV1",
  title: "Argus predictive replay v1",
});

export type PredictiveDatasetIndexV1 = typeof PredictiveDatasetIndexV1.Type;
export type PredictiveReplayV1 = typeof PredictiveReplayV1.Type;
