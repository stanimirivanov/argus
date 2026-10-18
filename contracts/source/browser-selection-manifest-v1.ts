import { Schema } from "effect";
import { ChangeReference } from "./execution-manifest-v1.js";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { CatalogSnapshotReference, Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));

const BrowserEvidenceReference = Schema.Struct({
  observationId: LocalKey,
  digest: Digest,
  collectorVersion: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  mappingRevision: Revision,
});

const BrowserDecision = Schema.Struct({
  testRepository: RepositoryReference,
  suiteKey: LocalKey,
  testKey: LocalKey,
  name: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255)),
  revision: Revision,
  projects: Schema.Array(Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127))),
  outcome: Schema.Literals(["RUN_REQUIRED", "SKIP_FOR_NOW"]),
  remainingExecution: Schema.Literals(["none", "full-suite"]),
  reason: Schema.Literals([
    "DIRECT_UI_MAPPING",
    "CONSERVATIVE_FALLBACK",
    "NOT_AFFECTED_IN_EARLY_STAGE",
  ]),
  mappingKeys: Schema.Array(Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127))),
});

/** Browser-owned decision; functional API v1 consumers must reject it. */
export const BrowserSelectionManifestV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-selection-manifest/v1"]),
  policyVersion: Schema.Literals(["argus.dev/selection-policy/browser-mapped/v1"]),
  change: ChangeReference,
  changedPaths: Schema.Array(
    Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(1024)),
  ).check(Schema.isMaxLength(10000)),
  pathsComplete: Schema.Boolean,
  catalog: CatalogSnapshotReference,
  evidence: Schema.NullOr(BrowserEvidenceReference),
  evaluatedAt: Timestamp,
  family: Schema.Literals(["functional-ui"]),
  mode: Schema.Literals(["targeted", "fallback"]),
  decisions: Schema.Array(BrowserDecision).check(Schema.isMaxLength(10000)),
  warnings: Schema.Array(Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(512))).check(
    Schema.isMaxLength(100),
  ),
}).annotate({
  identifier: "BrowserSelectionManifestV1",
  description:
    "Conservative browser selection bound to immutable discovery and reviewed UI mappings.",
});

export type BrowserSelectionManifestV1 = typeof BrowserSelectionManifestV1.Type;
