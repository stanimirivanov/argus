import { Schema } from "effect";

import { ExecutionManifestV1APIVersion } from "./execution-manifest-v1.js";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const SHA256 = Schema.String.pipe(Schema.pattern(/^[0-9a-f]{64}$/));
const PositiveTestCount = Schema.Int.pipe(
  Schema.greaterThanOrEqualTo(1),
  Schema.lessThanOrEqualTo(10000),
);
const ManifestReference = Schema.Struct({
  apiVersion: ExecutionManifestV1APIVersion,
  sha256: SHA256,
});

const ExecutionGroupBinding = Schema.Struct({
  groupKey: LocalKey,
  testRepository: RepositoryReference,
  testRevision: Revision,
  adapter: LocalKey,
});

const ExecutionJob = Schema.Struct({
  groupKey: LocalKey,
  stage: Schema.Literal("selected", "full-suite"),
  testRepository: RepositoryReference,
  testRevision: Revision,
  adapter: LocalKey,
  testCount: PositiveTestCount,
});

export const FunctionalAPIExecutionBindingsV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/functional-api-execution-bindings/v1"),
  groups: Schema.Array(ExecutionGroupBinding).pipe(Schema.maxItems(10000)),
}).annotations({
  identifier: "FunctionalAPIExecutionBindingsV1",
  title: "Argus functional API execution bindings v1",
  description:
    "Reviewed immutable test-repository revisions and stable group keys matched against a manifest.",
});

export const FunctionalAPIExecutionPlanV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/functional-api-execution-plan/v1"),
  manifest: ManifestReference,
  jobs: Schema.Array(ExecutionJob).pipe(Schema.maxItems(20000)),
}).annotations({
  identifier: "FunctionalAPIExecutionPlanV1",
  title: "Argus functional API execution plan v1",
  description:
    "Deterministic flattened selected and full-suite jobs for a CI matrix without executable commands.",
});

export type FunctionalAPIExecutionBindingsV1 = typeof FunctionalAPIExecutionBindingsV1.Type;
export type FunctionalAPIExecutionPlanV1 = typeof FunctionalAPIExecutionPlanV1.Type;
