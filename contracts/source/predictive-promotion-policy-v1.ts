import { Schema } from "effect";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Timestamp } from "./timestamp.js";

const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Positive = Schema.Int.check(
  Schema.isGreaterThanOrEqualTo(1),
  Schema.isLessThanOrEqualTo(Number.MAX_SAFE_INTEGER),
);
const Count = Schema.Int.check(
  Schema.isGreaterThanOrEqualTo(0),
  Schema.isLessThanOrEqualTo(Number.MAX_SAFE_INTEGER),
);
const Probability = Schema.Number.check(
  Schema.isGreaterThanOrEqualTo(0),
  Schema.isLessThanOrEqualTo(1),
);
const Family = Schema.Literals(["functional-api", "functional-ui"]);

/** Authenticated owner input; all thresholds are explicit, never inferred. */
export const PredictivePromotionPolicyV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-promotion-policy/v1"]),
  repository: RepositoryReference,
  family: Family,
  reviewId: LocalKey,
  reviewedBy: LocalKey,
  reviewedAt: Timestamp,
  expiresAt: Timestamp,
  scheduleSha256: Digest,
  modelSha256: Digest,
  calibrationSha256: Digest,
  maxEvidenceAgeSeconds: Positive.check(Schema.isLessThanOrEqualTo(31536000)),
  validForSeconds: Positive.check(Schema.isLessThanOrEqualTo(86400)),
  minReplayEpisodes: Positive,
  minShadowRuns: Positive,
  minFailures: Positive,
  maxMissRate: Probability,
  maxEce: Probability,
  maxEceDrift: Probability,
  maxStageDurationMs: Positive,
  maxStageResourceUnitMs: Positive,
});

const Measurement = Schema.Struct({
  samples: Count,
  failures: Count,
  misses: Count,
  missRate: Schema.NullOr(Probability),
});

/** A short-lived exact-portfolio choice; remaining execution is mandatory. */
export const PredictivePromotionDecisionV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-promotion-decision/v1"]),
  policySha256: Digest,
  replaySha256: Digest,
  shadowIndexSha256: Digest,
  scheduleSha256: Digest,
  modelSha256: Digest,
  calibrationSha256: Digest,
  repository: RepositoryReference,
  family: Family,
  evaluatedAt: Timestamp,
  expiresAt: Timestamp,
  mode: Schema.Literals(["shadow", "staged"]),
  remainingRequired: Schema.Literals([true]),
  reasons: Schema.Array(LocalKey).check(Schema.isMaxLength(30)),
  replay: Measurement,
  shadow: Measurement,
});
