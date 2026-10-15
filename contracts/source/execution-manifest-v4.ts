import { Schema } from "effect";
import { CapabilityImpactV2 } from "./capability-impact-v2.js";
import { ChangeReference } from "./execution-manifest-v1.js";
import { DisplayName, LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { CatalogSnapshotReference } from "./test-catalog-page-v1.js";

const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Reason = Schema.Struct({
  code: Schema.Literals([
    "DIRECT_CAPABILITY_IMPACT",
    "CONSERVATIVE_FALLBACK",
    "NOT_AFFECTED_IN_EARLY_STAGE",
    "MUST_RUN_RULE",
    "EXPLICIT_PIN",
    "NEW_OR_MODIFIED_TEST",
    "SUPPORTED_MAPPING",
    "EXCLUDED_EARLY_STAGE",
  ]),
  capabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(5000)),
  evidenceKeys: Schema.Array(LocalKey).check(Schema.isMaxLength(5000)),
});

const Decision = Schema.Struct({
  testRepository: RepositoryReference,
  suite: Schema.Struct({
    key: LocalKey,
    family: Schema.Literals(["functional-api"]),
    adapter: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  }),
  test: Schema.Struct({ key: LocalKey, name: DisplayName }),
  capabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(5000)),
  outcome: Schema.Literals(["RUN_REQUIRED", "SKIP_FOR_NOW"]),
  remainingExecution: Schema.Literals(["none", "full-suite"]),
  reasons: Schema.Array(Reason).check(Schema.isMinLength(1), Schema.isMaxLength(1)),
});

export const ExecutionManifestV4 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/execution-manifest/v4"]),
  policyVersion: Schema.Literals(["argus.dev/selection-policy/functional-api/v2"]),
  policyDigest: Digest,
  inventoryDigest: Digest,
  impact: Schema.Struct({
    apiVersion: CapabilityImpactV2.fields.apiVersion,
    analyzerVersion: CapabilityImpactV2.fields.analyzerVersion,
  }),
  change: ChangeReference,
  catalog: CatalogSnapshotReference,
  family: Schema.Literals(["functional-api"]),
  mode: Schema.Literals(["targeted", "fallback"]),
  affectedCapabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(5000)),
  uncoveredCapabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(5000)),
  decisions: Schema.Array(Decision).check(Schema.isMaxLength(10000)),
  warnings: Schema.Array(Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(512))).check(
    Schema.isMaxLength(100),
  ),
}).annotate({
  identifier: "ExecutionManifestV4",
  description:
    "Reviewed functional API policy with immutable policy, inventory and mapping evidence.",
});

export type ExecutionManifestV4 = typeof ExecutionManifestV4.Type;
