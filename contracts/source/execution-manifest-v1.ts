import { Schema } from "effect";
import { CapabilityImpactV1APIVersion } from "./capability-impact-v1.js";
import { DisplayName, LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { CatalogSnapshotReference, Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const PositiveInteger = Schema.Int.check(Schema.isGreaterThanOrEqualTo(1));

const ChangeReference = Schema.Struct({
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
});

const SelectionReason = Schema.Struct({
  code: Schema.Literals([
    "DIRECT_CAPABILITY_IMPACT",
    "CONSERVATIVE_FALLBACK",
    "NOT_AFFECTED_IN_EARLY_STAGE",
  ]),
  capabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(5000)),
});

const TestDecision = Schema.Struct({
  testRepository: RepositoryReference,
  suite: Schema.Struct({
    key: LocalKey,
    family: Schema.Literals(["functional-api"]),
    adapter: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  }),
  test: Schema.Struct({
    key: LocalKey,
    name: DisplayName,
  }),
  capabilities: Schema.Array(LocalKey).check(Schema.isMinLength(1), Schema.isMaxLength(5000)),
  outcome: Schema.Literals(["RUN_REQUIRED", "SKIP_FOR_NOW"]),
  remainingExecution: Schema.Literals(["none", "full-suite"]),
  reasons: Schema.Array(SelectionReason).check(Schema.isMinLength(1)),
});

export const ExecutionManifestV1APIVersion = Schema.Literals(["argus.dev/execution-manifest/v1"]);

export const ExecutionManifestV1 = Schema.Struct({
  apiVersion: ExecutionManifestV1APIVersion,
  policyVersion: Schema.Literals(["argus.dev/selection-policy/functional-api/v1"]),
  impact: Schema.Struct({
    apiVersion: CapabilityImpactV1APIVersion,
    analyzerVersion: Schema.Literals(["argus-openapi/v1+libopenapi/v0.38.7"]),
  }),
  change: ChangeReference,
  catalog: CatalogSnapshotReference,
  family: Schema.Literals(["functional-api"]),
  mode: Schema.Literals(["targeted", "fallback"]),
  affectedCapabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(5000)),
  uncoveredCapabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(5000)),
  decisions: Schema.Array(TestDecision).check(Schema.isMaxLength(10000)),
  warnings: Schema.Array(Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(512))).check(
    Schema.isMaxLength(100),
  ),
}).annotate({
  identifier: "ExecutionManifestV1",
  title: "Argus execution manifest v1",
  description:
    "Deterministic functional API inclusion and omission decisions from immutable impact and catalog inputs.",
});

export type ExecutionManifestV1 = typeof ExecutionManifestV1.Type;
