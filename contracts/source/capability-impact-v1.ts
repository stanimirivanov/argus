import { Schema } from "effect";

import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const PositiveInteger = Schema.Int.pipe(Schema.greaterThanOrEqualTo(1));
const NonNegativeInteger = Schema.Int.pipe(Schema.greaterThanOrEqualTo(0));
const RepositoryPath = Schema.String.pipe(Schema.minLength(1), Schema.maxLength(4096));
const OperationPath = Schema.String.pipe(
  Schema.minLength(1),
  Schema.maxLength(2048),
  Schema.pattern(/^\//),
);
const ChangeKind = Schema.Literal("added", "modified", "removed");

const OperationImpact = Schema.Struct({
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
  path: OperationPath,
  operationId: Schema.NullOr(Schema.String.pipe(Schema.minLength(1), Schema.maxLength(255))),
  kind: ChangeKind,
  capabilities: Schema.Array(LocalKey).pipe(Schema.maxItems(50)),
  potentiallyBreaking: Schema.Boolean,
}).annotations({
  identifier: "OperationImpact",
  description:
    "A semantically changed HTTP operation and its explicit repository-scoped capability mappings.",
});

const DocumentImpact = Schema.Struct({
  path: RepositoryPath,
  previousPath: Schema.NullOr(RepositoryPath),
  kind: ChangeKind,
  totalChanges: PositiveInteger,
  breakingChanges: NonNegativeInteger,
  operations: Schema.Array(OperationImpact).pipe(Schema.maxItems(5000)),
}).annotations({
  identifier: "OpenAPIDocumentImpact",
  description: "Semantic summary and operation evidence for one changed OpenAPI document.",
});

export const CapabilityImpactV1APIVersion = Schema.Literal("argus.dev/capability-impact/v1");

export const CapabilityImpactV1 = Schema.Struct({
  apiVersion: CapabilityImpactV1APIVersion,
  analyzerVersion: Schema.Literal("argus-openapi/v1+libopenapi/v0.38.7"),
  sourceRepository: RepositoryReference,
  pullRequest: Schema.Struct({ number: PositiveInteger }),
  baseRevision: Revision,
  headRevision: Revision,
  observedAt: Schema.DateTimeUtc,
  trigger: Schema.Struct({
    provider: Schema.Literal("github"),
    deliveryId: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(255)),
    event: Schema.Literal("pull_request"),
    action: Schema.Literal("opened", "reopened", "synchronize", "ready_for_review"),
  }),
  status: Schema.Literal("complete", "partial"),
  documents: Schema.Array(DocumentImpact).pipe(Schema.maxItems(50)),
  warnings: Schema.Array(Schema.String.pipe(Schema.minLength(1), Schema.maxLength(512))).pipe(
    Schema.maxItems(100),
  ),
}).annotations({
  identifier: "CapabilityImpactV1",
  title: "Argus capability impact v1",
  description:
    "Explainable capability impact derived from semantic OpenAPI changes at immutable revisions.",
});

export type CapabilityImpactV1 = typeof CapabilityImpactV1.Type;
