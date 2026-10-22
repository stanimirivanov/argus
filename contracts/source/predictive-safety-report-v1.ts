import { Schema } from "effect";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Count = Schema.Int.check(
  Schema.isGreaterThanOrEqualTo(0),
  Schema.isLessThanOrEqualTo(Number.MAX_SAFE_INTEGER),
);
const Probability = Schema.Number.check(
  Schema.isGreaterThanOrEqualTo(0),
  Schema.isLessThanOrEqualTo(1),
);
const Nullable = Schema.NullOr;
const Ref = Schema.Struct({ apiVersion: Schema.String, sha256: Digest });
const Mode = Schema.Literals(["shadow", "staged"]);
const File = Schema.Struct({
  path: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(1024)),
  sha256: Digest,
});

/** Explicit bounded archive inputs; paths are relative to the index directory. */
export const PredictiveSafetyIndexV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-safety-index/v1"]),
  windowStart: Timestamp,
  windowEnd: Timestamp,
  asOf: Timestamp,
  runs: Schema.Array(
    Schema.Struct({
      id: LocalKey,
      episodeId: LocalKey,
      mode: Mode,
      schedule: File,
      manifest: File,
      bindings: File,
      plan: File,
      discovery: Schema.optional(File),
      calibration: Schema.optional(File),
      attempts: Schema.Array(File).check(Schema.isMaxLength(20000)),
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(100)),
});

const Phase = Schema.Struct({
  stage: Schema.Literals(["relevant-now", "remaining-later", "shadow-control"]),
  state: Schema.Literals(["complete", "pending", "late", "empty", "incomplete"]),
  attemptId: Nullable(Schema.String),
  startedAt: Nullable(Timestamp),
  completedAt: Nullable(Timestamp),
  durationMs: Nullable(Count),
  elapsedMs: Nullable(Count),
  estimatedDurationMs: Count,
  estimatedResourceUnitMs: Count,
});
const CohortFields = {
  family: Schema.Literals(["functional-api", "functional-ui"]),
  selectionRate: Probability,
  failures: Nullable(Count),
  caughtFailures: Nullable(Count),
  recall: Nullable(Probability),
};

/** Observational measurements with explicit undefined values; no approval bit. */
export const PredictiveSafetyReportV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-safety-report/v1"]),
  inputSha256: Digest,
  windowStart: Timestamp,
  windowEnd: Timestamp,
  asOf: Timestamp,
  groups: Schema.Array(
    Schema.Struct({
      runId: LocalKey,
      episodeId: LocalKey,
      groupKey: LocalKey,
      mode: Mode,
      schedule: Ref,
      manifest: Ref,
      planSha256: Digest,
      policyVersion: Schema.String,
      testRepository: RepositoryReference,
      testRevision: Revision,
      discoverySha256: Nullable(Digest),
      featureCutoffAt: Timestamp,
      calibrationCutoffAt: Nullable(Timestamp),
      adapter: LocalKey,
      project: Schema.String,
      environment: LocalKey,
      calibration: Nullable(Ref),
      modelSha256: Nullable(Digest),
      baselineEce: Nullable(Probability),
      state: Schema.Literals(["complete", "pending", "incomplete"]),
      candidateCount: Count,
      earlyCount: Count,
      unknownLabels: Count,
      ...CohortFields,
      brier: Nullable(Probability),
      ece: Nullable(Probability),
      eceDrift: Nullable(
        Schema.Number.check(Schema.isGreaterThanOrEqualTo(-1), Schema.isLessThanOrEqualTo(1)),
      ),
      firstFailureMs: Nullable(Count),
      phases: Schema.Array(Phase).check(Schema.isMinLength(2), Schema.isMaxLength(2)),
      misses: Schema.Array(
        Schema.Struct({
          id: Digest,
          suiteKey: LocalKey,
          testKey: LocalKey,
          reason: Schema.Literals(["not-selected", "not-reproduced-early"]),
        }),
      ).check(Schema.isMaxLength(10000)),
      unavailable: Schema.Array(Schema.String).check(Schema.isMaxLength(20)),
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
  cohorts: Schema.Array(
    Schema.Struct({
      provider: Schema.String,
      host: Schema.String,
      providerRepositoryId: Schema.String,
      ...CohortFields,
      groups: Count,
      completeGroups: Count,
      candidates: Count,
      early: Count,
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
  unavailable: Schema.Array(Schema.String).check(Schema.isMaxLength(20)),
});
