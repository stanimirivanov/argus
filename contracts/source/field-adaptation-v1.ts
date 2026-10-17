import { Schema } from "effect";
import { AdaptationChangeReference, AdaptationTestReference } from "./adaptation-v1.js";
import { LocalKey } from "./repository-descriptor-v1.js";

const SHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Version = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127));
const FieldName = Schema.String.check(Schema.isPattern(/^[A-Za-z_$][A-Za-z0-9_$]{0,254}$/u));
const SourcePath = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(4096),
  Schema.isPattern(/^[^/\\][^\\]*$/u),
);
const OperationPath = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(2048),
  Schema.isPattern(/^\//u),
);

export const FieldRenameEvidenceV1 = Schema.Struct({
  documentPath: SourcePath,
  baseSha256: SHA256,
  headSha256: SHA256,
  method: Schema.Literals(["GET", "PUT", "POST", "DELETE", "PATCH"]),
  path: OperationPath,
  operationId: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255)),
  capabilities: Schema.Array(LocalKey).check(Schema.isMinLength(1), Schema.isMaxLength(50)),
  direction: Schema.Literals(["request", "response"]),
  statusCode: Schema.String.check(Schema.isMaxLength(3)),
  previousName: FieldName,
  name: FieldName,
});

export const FieldAdaptationTextEditV1 = Schema.Struct({
  path: SourcePath,
  beforeSha256: SHA256,
  startByte: Schema.Int.check(Schema.isGreaterThanOrEqualTo(0)),
  endByte: Schema.Int.check(Schema.isGreaterThan(0)),
  original: FieldName,
  replacement: FieldName,
  semanticRole: Schema.Literals(["request-body-key", "response-field-access"]),
});

export const FunctionalAPIFieldAdaptationRequestV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/functional-api-field-adaptation-request/v1"]),
  proposalId: SHA256,
  change: AdaptationChangeReference,
  test: AdaptationTestReference,
  fieldRename: FieldRenameEvidenceV1,
}).annotate({
  identifier: "FunctionalAPIFieldAdaptationRequestV1",
  title: "Argus functional API field adaptation request v1",
  description: "One immutable, compatible field rename sent to reviewed test-repository code.",
});

export const FunctionalAPIFieldAdaptationResultV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/functional-api-field-adaptation-result/v1"]),
  proposalId: SHA256,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  outcome: Schema.Literals(["candidate", "abstained"]),
  reasonCode: Schema.NullOr(LocalKey),
  reason: Schema.NullOr(Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(1000))),
  edit: Schema.NullOr(FieldAdaptationTextEditV1),
}).annotate({
  identifier: "FunctionalAPIFieldAdaptationResultV1",
  title: "Argus functional API field adaptation result v1",
  description: "One byte-addressed field reference edit or explicit abstention.",
});

export const FieldAdaptationProposalV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/field-adaptation-proposal/v1"]),
  policyVersion: Schema.Literals([
    "argus.dev/adaptation-policy/functional-api-compatible-field-rename/v1",
  ]),
  proposalId: SHA256,
  sourceImpact: Schema.Struct({
    apiVersion: Schema.Literals(["argus.dev/capability-impact/v1"]),
    analyzerVersion: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  }),
  change: AdaptationChangeReference,
  test: AdaptationTestReference,
  classification: Schema.Literals(["INVALIDATED"]),
  decision: Schema.Literals(["PATCH_AND_VALIDATE"]),
  fieldRename: FieldRenameEvidenceV1,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  edit: FieldAdaptationTextEditV1,
}).annotate({
  identifier: "FieldAdaptationProposalV1",
  title: "Argus field adaptation proposal v1",
  description:
    "Reviewable proposal only; field-specific validation is required before publication.",
});
