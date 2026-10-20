import { Schema } from "effect";
import { ChangeReference } from "./execution-manifest-v1.js";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Count = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0));
const Duration = Count.check(Schema.isLessThanOrEqualTo(86_400_000));
const File = Schema.Struct({
  path: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(4096)),
  kind: Schema.Literals(["added", "modified", "deleted", "renamed", "copied"]),
  additions: Count,
  deletions: Count,
  patchStatus: Schema.Literals(["complete", "unavailable", "truncated", "budget-exhausted"]),
});
const Test = Schema.Struct({
  suiteKey: LocalKey,
  testKey: LocalKey,
  decision: Schema.Literals(["RUN_REQUIRED", "SKIP_FOR_NOW"]),
  reasons: Schema.Array(Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127))).check(
    Schema.isMaxLength(100),
  ),
  capabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(5000)),
  mappingKeys: Schema.Array(
    Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  ).check(Schema.isMaxLength(5000)),
  outcome: Schema.Literals(["passed", "failed"]),
  durationMs: Duration,
});

/** One explicitly bound full-control group; outcome labels are separate from decision-time features. */
export const PredictiveObservationV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-observation/v1"]),
  change: ChangeReference,
  files: Schema.Array(File).check(Schema.isMaxLength(1000)),
  filesTruncated: Schema.Boolean,
  manifest: Schema.Struct({
    apiVersion: Schema.Literals([
      "argus.dev/execution-manifest/v1",
      "argus.dev/execution-manifest/v4",
      "argus.dev/browser-selection-manifest/v1",
    ]),
    sha256: Digest,
  }),
  featureCutoffAt: Timestamp,
  family: Schema.Literals(["functional-api", "functional-ui"]),
  testRepository: RepositoryReference,
  testRevision: Revision,
  adapter: LocalKey,
  project: Schema.NullOr(LocalKey),
  environment: Schema.NullOr(LocalKey),
  controlAttemptId: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  controlStartedAt: Timestamp,
  controlCompletedAt: Timestamp,
  tests: Schema.Array(Test).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
}).annotate({
  identifier: "PredictiveObservationV1",
  title: "Argus predictive observation v1",
  description: "Immutable decision-time features and full-suite labels for one execution group.",
});

export const PredictiveObservationEnvelopeV1 = Schema.Struct({
  observation: PredictiveObservationV1,
  sha256: Digest,
}).annotate({
  identifier: "PredictiveObservationEnvelopeV1",
  title: "Argus predictive observation envelope v1",
  description:
    "Canonical observation with SHA-256 of its JSON payload, excluding this envelope field.",
});

export type PredictiveObservationV1 = typeof PredictiveObservationV1.Type;
export type PredictiveObservationEnvelopeV1 = typeof PredictiveObservationEnvelopeV1.Type;
