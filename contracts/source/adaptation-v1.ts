import { Schema } from "effect";

import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const SHA256 = Schema.String.pipe(Schema.pattern(/^[0-9a-f]{64}$/));
const SourcePath = Schema.String.pipe(
  Schema.minLength(1),
  Schema.maxLength(4096),
  Schema.pattern(/^[^/\\][^\\]*$/),
);
const OperationPath = Schema.String.pipe(
  Schema.minLength(1),
  Schema.maxLength(2048),
  Schema.pattern(/^\//),
);
const ByteOffset = Schema.Int.pipe(Schema.greaterThanOrEqualTo(0));
const Version = Schema.String.pipe(Schema.minLength(1), Schema.maxLength(127));

const ChangeReference = Schema.Struct({
  sourceRepository: RepositoryReference,
  pullRequest: Schema.Struct({ number: Schema.Int.pipe(Schema.greaterThanOrEqualTo(1)) }),
  baseRevision: Revision,
  headRevision: Revision,
  observedAt: Schema.DateTimeUtc,
  trigger: Schema.Struct({
    provider: Schema.Literal("github"),
    deliveryId: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(255)),
    event: Schema.Literal("pull_request"),
    action: Schema.Literal("opened", "reopened", "synchronize", "ready_for_review"),
  }),
});

export const AdaptationTestReference = Schema.Struct({
  repository: RepositoryReference,
  revision: Revision,
  suiteKey: LocalKey,
  testKey: LocalKey,
  name: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(255)),
  adapter: LocalKey,
  capabilities: Schema.Array(LocalKey).pipe(Schema.minItems(1), Schema.maxItems(5000)),
});

const EndpointRename = Schema.Struct({
  method: Schema.Literal(
    "GET",
    "PUT",
    "POST",
    "DELETE",
    "OPTIONS",
    "HEAD",
    "PATCH",
    "TRACE",
    "QUERY",
  ),
  operationId: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(255)),
  previousPath: OperationPath,
  path: OperationPath,
  capabilities: Schema.Array(LocalKey).pipe(Schema.minItems(1), Schema.maxItems(50)),
});

export const AdaptationTextEdit = Schema.Struct({
  path: SourcePath,
  beforeSha256: SHA256,
  startByte: ByteOffset,
  endByte: ByteOffset,
  original: OperationPath,
  replacement: OperationPath,
  semanticRole: Schema.Literal("request-target"),
});

export const FunctionalAPIAdaptationRequestV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/functional-api-adaptation-request/v1"),
  proposalId: SHA256,
  change: ChangeReference,
  test: AdaptationTestReference,
  endpointRename: EndpointRename,
}).annotations({
  identifier: "FunctionalAPIAdaptationRequestV1",
  title: "Argus functional API adaptation request v1",
  description:
    "Authoritative endpoint rename evidence and one immutable test supplied to a reviewed source adapter.",
});

export const FunctionalAPIAdaptationResultV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/functional-api-adaptation-result/v1"),
  proposalId: SHA256,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  outcome: Schema.Literal("candidate", "abstained"),
  reasonCode: Schema.NullOr(LocalKey),
  reason: Schema.NullOr(Schema.String.pipe(Schema.minLength(1), Schema.maxLength(1000))),
  edit: Schema.NullOr(AdaptationTextEdit),
}).annotations({
  identifier: "FunctionalAPIAdaptationResultV1",
  title: "Argus functional API adaptation result v1",
  description:
    "One untrusted request-target edit or explicit abstention returned by a reviewed framework adapter.",
});

export const AdaptationProposalV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/adaptation-proposal/v1"),
  policyVersion: Schema.Literal("argus.dev/adaptation-policy/functional-api-endpoint-rename/v1"),
  proposalId: SHA256,
  sourceImpact: Schema.Struct({
    apiVersion: Schema.Literal("argus.dev/capability-impact/v1"),
    analyzerVersion: Schema.Literal("argus-openapi/v1+libopenapi/v0.38.7"),
  }),
  change: ChangeReference,
  test: AdaptationTestReference,
  classification: Schema.Literal("INVALIDATED"),
  decision: Schema.Literal("PATCH_AND_VALIDATE"),
  endpointRename: EndpointRename,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  edit: AdaptationTextEdit,
}).annotations({
  identifier: "AdaptationProposalV1",
  title: "Argus adaptation proposal v1",
  description:
    "A deterministic, reviewable request-target edit that still requires isolated validation.",
});

export type FunctionalAPIAdaptationRequestV1 = typeof FunctionalAPIAdaptationRequestV1.Type;
export type FunctionalAPIAdaptationResultV1 = typeof FunctionalAPIAdaptationResultV1.Type;
export type AdaptationProposalV1 = typeof AdaptationProposalV1.Type;
