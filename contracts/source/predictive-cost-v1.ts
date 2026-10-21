import { Schema } from "effect";
import { LocalKey } from "./repository-descriptor-v1.js";
import { Timestamp } from "./timestamp.js";

const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Count = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0));
const Duration = Count.check(Schema.isLessThanOrEqualTo(86400000));
const Total = Count.check(Schema.isLessThanOrEqualTo(8640000000000000));
const Units = Schema.Int.check(Schema.isGreaterThanOrEqualTo(1), Schema.isLessThanOrEqualTo(10000));
const EnvironmentFields = {
  key: LocalKey,
  capacityUnits: Units,
  setupDurationMs: Duration,
};
const TestFields = {
  id: Digest,
  environment: LocalKey,
  resourceUnits: Units,
  coldStartDurationMs: Duration.check(Schema.isGreaterThan(0)),
  dependencies: Schema.Array(Digest).check(Schema.isMaxLength(100)),
};
const ReviewFields = {
  id: LocalKey,
  reviewId: LocalKey,
  reviewedBy: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  reviewedAt: Timestamp,
  featureCutoffAt: Timestamp,
  manifestSha256: Digest,
};

/** Complete decision-time declarations; provenance authenticity belongs to source CI. */
export const PredictiveStageConstraintsV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-stage-constraints/v1"]),
  episodes: Schema.Array(
    Schema.Struct({
      ...ReviewFields,
      environments: Schema.Array(Schema.Struct(EnvironmentFields)).check(
        Schema.isMinLength(1),
        Schema.isMaxLength(100),
      ),
      tests: Schema.Array(Schema.Struct(TestFields)).check(
        Schema.isMinLength(1),
        Schema.isMaxLength(10000),
      ),
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(1000)),
}).annotate({
  identifier: "PredictiveStageConstraintsV1",
  title: "Argus reviewed predictive stage constraints v1",
});

/** Estimates and lower bounds over the complete candidate set, not a schedule. */
export const PredictiveStageCostV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-stage-cost/v1"]),
  constraintsSha256: Digest,
  episodes: Schema.Array(
    Schema.Struct({
      ...ReviewFields,
      environments: Schema.Array(
        Schema.Struct({
          ...EnvironmentFields,
          workUnitMs: Total,
          resourceLowerBoundMs: Total,
        }),
      ).check(Schema.isMinLength(1), Schema.isMaxLength(100)),
      tests: Schema.Array(
        Schema.Struct({
          ...TestFields,
          durationMs: Duration.check(Schema.isGreaterThan(0)),
          samples: Count.check(Schema.isLessThanOrEqualTo(1000)),
          source: Schema.Literals(["training-p90", "reviewed-cold-start"]),
        }),
      ).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
      serialDurationMs: Total,
      dependencyLowerBoundMs: Total,
      lowerBoundMs: Total,
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(1000)),
}).annotate({ identifier: "PredictiveStageCostV1", title: "Argus predictive stage cost v1" });

export type PredictiveStageConstraintsV1 = typeof PredictiveStageConstraintsV1.Type;
export type PredictiveStageCostV1 = typeof PredictiveStageCostV1.Type;
