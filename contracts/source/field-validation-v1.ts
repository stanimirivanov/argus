import { Schema } from "effect";
import { AdaptationTestReference } from "./adaptation-v1.js";
import { FieldAdaptationTextEditV1 } from "./field-adaptation-v1.js";
import { LocalKey } from "./repository-descriptor-v1.js";
import { Timestamp } from "./timestamp.js";

const SHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Phase = Schema.Literals(["original", "candidate", "negative-control"]);
const Outcome = Schema.Literals(["passed", "failed", "error"]);
const Failure = Schema.Struct({
  code: LocalKey,
  message: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(2000)),
});
const Run = Schema.Struct({
  phase: Phase,
  repeat: Schema.Int.check(Schema.isGreaterThanOrEqualTo(1), Schema.isLessThanOrEqualTo(2)),
  sourceSha256: SHA256,
  startedAt: Timestamp,
  completedAt: Timestamp,
  outcome: Schema.Literals(["passed", "failed"]),
  failure: Schema.NullOr(Failure),
});
const Source = Schema.Struct({
  path: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(4096)),
  originalSha256: SHA256,
  candidateSha256: SHA256,
  negativeSha256: SHA256,
  restoredSha256: SHA256,
  negativeControlField: Schema.String.check(
    Schema.isPattern(/^__argus_negative_field_[0-9a-f]{12}$/u),
  ),
});
const Common = {
  policyVersion: Schema.Literals([
    "argus.dev/validation-policy/functional-api-compatible-field-rename/v1",
  ]),
  validationId: SHA256,
  proposalId: SHA256,
  proposalPolicyVersion: Schema.Literals([
    "argus.dev/adaptation-policy/functional-api-compatible-field-rename/v1",
  ]),
  test: AdaptationTestReference,
  adapter: Schema.Struct({
    id: LocalKey,
    version: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  }),
  edit: FieldAdaptationTextEditV1,
  source: Source,
};

export const FunctionalAPIFieldValidationRequestV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/functional-api-field-validation-request/v1"]),
  validationId: SHA256,
  proposalId: SHA256,
  phase: Phase,
  repeat: Schema.Int.check(Schema.isGreaterThanOrEqualTo(1), Schema.isLessThanOrEqualTo(2)),
  test: AdaptationTestReference,
  sourceSha256: SHA256,
}).annotate({ identifier: "FunctionalAPIFieldValidationRequestV1" });

export const FunctionalAPIFieldValidationResultV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/functional-api-field-validation-result/v1"]),
  validationId: SHA256,
  proposalId: SHA256,
  phase: Phase,
  repeat: Schema.Int.check(Schema.isGreaterThanOrEqualTo(1), Schema.isLessThanOrEqualTo(2)),
  suiteKey: LocalKey,
  testKey: LocalKey,
  adapter: Schema.Struct({
    id: LocalKey,
    version: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  }),
  sourceSha256: SHA256,
  startedAt: Timestamp,
  completedAt: Timestamp,
  outcome: Outcome,
  failure: Schema.NullOr(Failure),
}).annotate({ identifier: "FunctionalAPIFieldValidationResultV1" });

export const FieldValidationEvidenceV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/field-validation-evidence/v1"]),
  ...Common,
  runs: Schema.Array(Run).check(Schema.isMinLength(6), Schema.isMaxLength(6)),
}).annotate({ identifier: "FieldValidationEvidenceV1" });

export const FieldValidationRejectionV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/field-validation-rejection/v1"]),
  ...Common,
  rejection: Schema.Struct({
    phase: Phase,
    reason: Schema.Literals(["original-passed", "candidate-failed", "negative-control-passed"]),
    expectedOutcome: Schema.Literals(["passed", "failed"]),
    actualOutcome: Schema.Literals(["passed", "failed"]),
  }),
  runs: Schema.Array(Run).check(Schema.isMinLength(2), Schema.isMaxLength(6)),
}).annotate({ identifier: "FieldValidationRejectionV1" });
