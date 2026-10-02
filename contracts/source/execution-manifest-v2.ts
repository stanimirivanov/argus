import { Schema } from "effect";

import { CapabilityImpactV1APIVersion } from "./capability-impact-v1.js";
import { DisplayName, LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { CatalogSnapshotReference, Revision } from "./test-catalog-page-v1.js";

const PositiveInteger = Schema.Int.pipe(Schema.greaterThanOrEqualTo(1));

const ChangeReference = Schema.Struct({
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
});

const SelectionReason = Schema.Struct({
  code: Schema.Literal(
    "DIRECT_CAPABILITY_IMPACT",
    "CONSERVATIVE_FALLBACK",
    "NOT_AFFECTED_IN_EARLY_STAGE",
  ),
  capabilities: Schema.Array(LocalKey).pipe(Schema.maxItems(5000)),
});

const TestDecision = Schema.Struct({
  testRepository: RepositoryReference,
  suite: Schema.Struct({
    key: LocalKey,
    family: Schema.Literal("functional-ui"),
    adapter: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(127)),
  }),
  test: Schema.Struct({ key: LocalKey, name: DisplayName }),
  capabilities: Schema.Array(LocalKey).pipe(Schema.minItems(1), Schema.maxItems(5000)),
  outcome: Schema.Literal("RUN_REQUIRED", "SKIP_FOR_NOW"),
  remainingExecution: Schema.Literal("none", "full-suite"),
  reasons: Schema.Array(SelectionReason).pipe(Schema.minItems(1)),
});

export const ExecutionManifestV2APIVersion = Schema.Literal("argus.dev/execution-manifest/v2");

export const ExecutionManifestV2 = Schema.Struct({
  apiVersion: ExecutionManifestV2APIVersion,
  policyVersion: Schema.Literal("argus.dev/selection-policy/functional-ui-capability/v1"),
  impact: Schema.Struct({
    apiVersion: CapabilityImpactV1APIVersion,
    analyzerVersion: Schema.Literal("argus-openapi/v1+libopenapi/v0.38.7"),
  }),
  change: ChangeReference,
  catalog: CatalogSnapshotReference,
  family: Schema.Literal("functional-ui"),
  mode: Schema.Literal("targeted", "fallback"),
  affectedCapabilities: Schema.Array(LocalKey).pipe(Schema.maxItems(5000)),
  uncoveredCapabilities: Schema.Array(LocalKey).pipe(Schema.maxItems(5000)),
  decisions: Schema.Array(TestDecision).pipe(Schema.maxItems(10000)),
  warnings: Schema.Array(Schema.String.pipe(Schema.minLength(1), Schema.maxLength(512))).pipe(
    Schema.maxItems(100),
  ),
}).annotations({
  identifier: "ExecutionManifestV2",
  title: "Argus browser capability selection manifest v2",
  description:
    "Deterministic functional UI inclusion and omission decisions from immutable OpenAPI capability impact and catalog inputs.",
});

export type ExecutionManifestV2 = typeof ExecutionManifestV2.Type;
