import { Schema } from "effect";

import { AdaptationTestReference, AdaptationTextEdit } from "./adaptation-v1.js";
import { LocalKey } from "./repository-descriptor-v1.js";

const SHA256 = Schema.String.pipe(Schema.pattern(/^[0-9a-f]{64}$/));
const Version = Schema.String.pipe(Schema.minLength(1), Schema.maxLength(127));
const SourcePath = Schema.String.pipe(
  Schema.minLength(1),
  Schema.maxLength(4096),
  Schema.pattern(/^[^/\\][^\\]*$/),
);
const Phase = Schema.Literal("original", "candidate", "negative-control");
const Failure = Schema.Struct({
  code: LocalKey,
  message: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(2000)),
});

export const FunctionalAPIRepairValidationRequestV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/functional-api-repair-validation-request/v1"),
  validationId: SHA256,
  proposalId: SHA256,
  phase: Phase,
  test: AdaptationTestReference,
  sourceSha256: SHA256,
}).annotations({
  identifier: "FunctionalAPIRepairValidationRequestV1",
  title: "Argus functional API repair validation request v1",
  description:
    "One exact test and disposable-workspace phase supplied to a reviewed validation adapter.",
});

export const FunctionalAPIRepairValidationResultV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/functional-api-repair-validation-result/v1"),
  validationId: SHA256,
  proposalId: SHA256,
  phase: Phase,
  suiteKey: LocalKey,
  testKey: LocalKey,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  sourceSha256: SHA256,
  startedAt: Schema.DateTimeUtc,
  completedAt: Schema.DateTimeUtc,
  outcome: Schema.Literal("passed", "failed", "error"),
  failure: Schema.NullOr(Failure),
}).annotations({
  identifier: "FunctionalAPIRepairValidationResultV1",
  title: "Argus functional API repair validation result v1",
  description: "Normalized result for one exact adaptation validation phase.",
});

const ValidationRun = Schema.Struct({
  phase: Phase,
  sourceSha256: SHA256,
  startedAt: Schema.DateTimeUtc,
  completedAt: Schema.DateTimeUtc,
  outcome: Schema.Literal("passed", "failed"),
  failure: Schema.NullOr(Failure),
});

export const ValidationEvidenceV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/validation-evidence/v1"),
  policyVersion: Schema.Literal("argus.dev/validation-policy/functional-api-endpoint-rename/v1"),
  validationId: SHA256,
  proposalId: SHA256,
  proposalPolicyVersion: Schema.Literal(
    "argus.dev/adaptation-policy/functional-api-endpoint-rename/v1",
  ),
  test: AdaptationTestReference,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  edit: AdaptationTextEdit,
  source: Schema.Struct({
    path: SourcePath,
    originalSha256: SHA256,
    candidateSha256: SHA256,
    negativeSha256: SHA256,
    restoredSha256: SHA256,
    negativeControlPath: Schema.String.pipe(
      Schema.minLength(1),
      Schema.maxLength(2048),
      Schema.pattern(/^\/__argus_negative_control__\//),
    ),
  }),
  runs: Schema.Array(ValidationRun).pipe(Schema.minItems(3), Schema.maxItems(3)),
}).annotations({
  identifier: "ValidationEvidenceV1",
  title: "Argus validation evidence v1",
  description:
    "Successful original-failure, repaired-success, negative-control-failure evidence for one proposal.",
});

export const ValidationRejectionV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/validation-rejection/v1"),
  policyVersion: Schema.Literal("argus.dev/validation-policy/functional-api-endpoint-rename/v1"),
  validationId: SHA256,
  proposalId: SHA256,
  proposalPolicyVersion: Schema.Literal(
    "argus.dev/adaptation-policy/functional-api-endpoint-rename/v1",
  ),
  test: AdaptationTestReference,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  edit: AdaptationTextEdit,
  source: Schema.Struct({
    path: SourcePath,
    originalSha256: SHA256,
    candidateSha256: SHA256,
    negativeSha256: SHA256,
    restoredSha256: SHA256,
    negativeControlPath: Schema.String.pipe(
      Schema.minLength(1),
      Schema.maxLength(2048),
      Schema.pattern(/^\/__argus_negative_control__\//),
    ),
  }),
  rejection: Schema.Struct({
    phase: Phase,
    reason: Schema.Literal("original-passed", "candidate-failed", "negative-control-passed"),
    expectedOutcome: Schema.Literal("passed", "failed"),
    actualOutcome: Schema.Literal("passed", "failed"),
  }),
  runs: Schema.Array(ValidationRun).pipe(Schema.minItems(1), Schema.maxItems(3)),
}).annotations({
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
