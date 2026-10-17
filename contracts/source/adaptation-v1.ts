import { Schema } from "effect";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const SHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
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
const ByteOffset = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0));
const Version = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127));

export const AdaptationChangeReference = Schema.Struct({
  sourceRepository: RepositoryReference,
  pullRequest: Schema.Struct({ number: Schema.Int.check(Schema.isGreaterThanOrEqualTo(1)) }),
  baseRevision: Revision,
  headRevision: Revision,
  observedAt: Timestamp,
  trigger: Schema.Struct({
    provider: Schema.Literals(["github"]),
    deliveryId: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255)),
    event: Schema.Literals(["pull_request"]),
    action: Schema.Literals(["opened", "reopened", "synchronize", "ready_for_review"]),
  }),
});

export const AdaptationTestReference = Schema.Struct({
  repository: RepositoryReference,
  revision: Revision,
  suiteKey: LocalKey,
  testKey: LocalKey,
  name: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255)),
  adapter: LocalKey,
  capabilities: Schema.Array(LocalKey).check(Schema.isMinLength(1), Schema.isMaxLength(5000)),
});

const EndpointRename = Schema.Struct({
  method: Schema.Literals([
    "GET",
    "PUT",
    "POST",
    "DELETE",
    "OPTIONS",
    "HEAD",
    "PATCH",
    "TRACE",
    "QUERY",
  ]),
  operationId: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255)),
  previousPath: OperationPath,
  path: OperationPath,
  capabilities: Schema.Array(LocalKey).check(Schema.isMinLength(1), Schema.isMaxLength(50)),
});

export const AdaptationTextEdit = Schema.Struct({
  path: SourcePath,
  beforeSha256: SHA256,
  startByte: ByteOffset,
  endByte: ByteOffset,
  original: OperationPath,
  replacement: OperationPath,
  semanticRole: Schema.Literals(["request-target"]),
});

export const FunctionalAPIAdaptationRequestV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/functional-api-adaptation-request/v1"]),
  proposalId: SHA256,
  change: AdaptationChangeReference,
  test: AdaptationTestReference,
  endpointRename: EndpointRename,
}).annotate({
  identifier: "FunctionalAPIAdaptationRequestV1",
  title: "Argus functional API adaptation request v1",
  description:
    "Authoritative endpoint rename evidence and one immutable test supplied to a reviewed source adapter.",
});

export const FunctionalAPIAdaptationResultV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/functional-api-adaptation-result/v1"]),
  proposalId: SHA256,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  outcome: Schema.Literals(["candidate", "abstained"]),
  reasonCode: Schema.NullOr(LocalKey),
  reason: Schema.NullOr(Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(1000))),
  edit: Schema.NullOr(AdaptationTextEdit),
}).annotate({
  identifier: "FunctionalAPIAdaptationResultV1",
  title: "Argus functional API adaptation result v1",
  description:
    "One untrusted request-target edit or explicit abstention returned by a reviewed framework adapter.",
});

export const AdaptationProposalV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/adaptation-proposal/v1"]),
  policyVersion: Schema.Literals(["argus.dev/adaptation-policy/functional-api-endpoint-rename/v1"]),
  proposalId: SHA256,
  sourceImpact: Schema.Struct({
    apiVersion: Schema.Literals(["argus.dev/capability-impact/v1"]),
    analyzerVersion: Schema.Literals(["argus-openapi/v1+libopenapi/v0.38.7"]),
  }),
  change: AdaptationChangeReference,
  test: AdaptationTestReference,
  classification: Schema.Literals(["INVALIDATED"]),
  decision: Schema.Literals(["PATCH_AND_VALIDATE"]),
  endpointRename: EndpointRename,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  edit: AdaptationTextEdit,
}).annotate({
  identifier: "AdaptationProposalV1",
  title: "Argus adaptation proposal v1",
  description:
    "A deterministic, reviewable request-target edit that still requires isolated validation.",
});

export type FunctionalAPIAdaptationRequestV1 = typeof FunctionalAPIAdaptationRequestV1.Type;
export type FunctionalAPIAdaptationResultV1 = typeof FunctionalAPIAdaptationResultV1.Type;
export type AdaptationProposalV1 = typeof AdaptationProposalV1.Type;
