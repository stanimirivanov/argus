import { Schema } from "effect";
import { AdaptationTestReference, AdaptationTextEdit } from "./adaptation-v1.js";
import { LocalKey } from "./repository-descriptor-v1.js";
import { Timestamp } from "./timestamp.js";

const SHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Version = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127));
const SourcePath = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(4096),
  Schema.isPattern(/^[^/\\][^\\]*$/u),
);
const Phase = Schema.Literals(["original", "candidate", "negative-control"]);
const Failure = Schema.Struct({
  code: LocalKey,
  message: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(2000)),
});

export const FunctionalAPIRepairValidationRequestV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/functional-api-repair-validation-request/v1"]),
  validationId: SHA256,
  proposalId: SHA256,
  phase: Phase,
  test: AdaptationTestReference,
  sourceSha256: SHA256,
}).annotate({
  identifier: "FunctionalAPIRepairValidationRequestV1",
  title: "Argus functional API repair validation request v1",
  description:
    "One exact test and disposable-workspace phase supplied to a reviewed validation adapter.",
});

export const FunctionalAPIRepairValidationResultV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/functional-api-repair-validation-result/v1"]),
  validationId: SHA256,
  proposalId: SHA256,
  phase: Phase,
  suiteKey: LocalKey,
  testKey: LocalKey,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  sourceSha256: SHA256,
  startedAt: Timestamp,
  completedAt: Timestamp,
  outcome: Schema.Literals(["passed", "failed", "error"]),
  failure: Schema.NullOr(Failure),
}).annotate({
  identifier: "FunctionalAPIRepairValidationResultV1",
  title: "Argus functional API repair validation result v1",
  description: "Normalized result for one exact adaptation validation phase.",
});

const ValidationRun = Schema.Struct({
  phase: Phase,
  sourceSha256: SHA256,
  startedAt: Timestamp,
  completedAt: Timestamp,
  outcome: Schema.Literals(["passed", "failed"]),
  failure: Schema.NullOr(Failure),
});

export const ValidationEvidenceV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/validation-evidence/v1"]),
  policyVersion: Schema.Literals(["argus.dev/validation-policy/functional-api-endpoint-rename/v1"]),
  validationId: SHA256,
  proposalId: SHA256,
  proposalPolicyVersion: Schema.Literals([
    "argus.dev/adaptation-policy/functional-api-endpoint-rename/v1",
  ]),
  test: AdaptationTestReference,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  edit: AdaptationTextEdit,
  source: Schema.Struct({
    path: SourcePath,
    originalSha256: SHA256,
    candidateSha256: SHA256,
    negativeSha256: SHA256,
    restoredSha256: SHA256,
    negativeControlPath: Schema.String.check(
      Schema.isMinLength(1),
      Schema.isMaxLength(2048),
      Schema.isPattern(/^\/__argus_negative_control__\//u),
    ),
  }),
  runs: Schema.Array(ValidationRun).check(Schema.isMinLength(3), Schema.isMaxLength(3)),
}).annotate({
  identifier: "ValidationEvidenceV1",
  title: "Argus validation evidence v1",
  description:
    "Successful original-failure, repaired-success, negative-control-failure evidence for one proposal.",
});

export const ValidationRejectionV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/validation-rejection/v1"]),
  policyVersion: Schema.Literals(["argus.dev/validation-policy/functional-api-endpoint-rename/v1"]),
  validationId: SHA256,
  proposalId: SHA256,
  proposalPolicyVersion: Schema.Literals([
    "argus.dev/adaptation-policy/functional-api-endpoint-rename/v1",
  ]),
  test: AdaptationTestReference,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  edit: AdaptationTextEdit,
  source: Schema.Struct({
    path: SourcePath,
    originalSha256: SHA256,
    candidateSha256: SHA256,
    negativeSha256: SHA256,
    restoredSha256: SHA256,
    negativeControlPath: Schema.String.check(
      Schema.isMinLength(1),
      Schema.isMaxLength(2048),
      Schema.isPattern(/^\/__argus_negative_control__\//u),
    ),
  }),
  rejection: Schema.Struct({
    phase: Phase,
    reason: Schema.Literals(["original-passed", "candidate-failed", "negative-control-passed"]),
    expectedOutcome: Schema.Literals(["passed", "failed"]),
    actualOutcome: Schema.Literals(["passed", "failed"]),
  }),
  runs: Schema.Array(ValidationRun).check(Schema.isMinLength(1), Schema.isMaxLength(3)),
}).annotate({
  identifier: "ValidationRejectionV1",
  title: "Argus validation rejection v1",
  description:
    "A trustworthy completed validation prefix that disproves a repair candidate without treating infrastructure errors as policy evidence.",
});

export type FunctionalAPIRepairValidationRequestV1 =
  typeof FunctionalAPIRepairValidationRequestV1.Type;
export type FunctionalAPIRepairValidationResultV1 =
  typeof FunctionalAPIRepairValidationResultV1.Type;
export type ValidationEvidenceV1 = typeof ValidationEvidenceV1.Type;
export type ValidationRejectionV1 = typeof ValidationRejectionV1.Type;
