import { Schema } from "effect";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const PositiveInteger = Schema.Int.check(Schema.isGreaterThanOrEqualTo(1));
const NonNegativeInteger = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0));
const RepositoryPath = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(4096));
const OperationPath = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(2048),
  Schema.isPattern(/^\//u),
);
const ChangeKind = Schema.Literals(["added", "modified", "removed"]);

const OperationImpact = Schema.Struct({
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
  path: OperationPath,
  operationId: Schema.NullOr(Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255))),
  kind: ChangeKind,
  capabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(50)),
  potentiallyBreaking: Schema.Boolean,
}).annotate({
  identifier: "OperationImpact",
  description:
    "A semantically changed HTTP operation and its explicit repository-scoped capability mappings.",
});

export const DocumentImpact = Schema.Struct({
  path: RepositoryPath,
  previousPath: Schema.NullOr(RepositoryPath),
  kind: ChangeKind,
  totalChanges: PositiveInteger,
  breakingChanges: NonNegativeInteger,
  operations: Schema.Array(OperationImpact).check(Schema.isMaxLength(5000)),
}).annotate({
  identifier: "OpenAPIDocumentImpact",
  description: "Semantic summary and operation evidence for one changed OpenAPI document.",
});

export const CapabilityImpactV1APIVersion = Schema.Literals(["argus.dev/capability-impact/v1"]);

export const CapabilityImpactV1 = Schema.Struct({
  apiVersion: CapabilityImpactV1APIVersion,
  analyzerVersion: Schema.Literals(["argus-openapi/v1+libopenapi/v0.38.7"]),
  sourceRepository: RepositoryReference,
  pullRequest: Schema.Struct({ number: PositiveInteger }),
  baseRevision: Revision,
  headRevision: Revision,
  observedAt: Timestamp,
  trigger: Schema.Struct({
    provider: Schema.Literals(["github"]),
    deliveryId: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255)),
    event: Schema.Literals(["pull_request"]),
    action: Schema.Literals(["opened", "reopened", "synchronize", "ready_for_review"]),
  }),
  status: Schema.Literals(["complete", "partial"]),
  documents: Schema.Array(DocumentImpact).check(Schema.isMaxLength(50)),
  warnings: Schema.Array(Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(512))).check(
    Schema.isMaxLength(100),
  ),
}).annotate({
  identifier: "CapabilityImpactV1",
  title: "Argus capability impact v1",
  description:
    "Explainable capability impact derived from semantic OpenAPI changes at immutable revisions.",
});

export type CapabilityImpactV1 = typeof CapabilityImpactV1.Type;
