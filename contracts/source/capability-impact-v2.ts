import { Schema } from "effect";
import { DocumentImpact } from "./capability-impact-v1.js";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const RepositoryPath = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(4096));
const FileImpact = Schema.Struct({
  path: RepositoryPath,
  previousPath: Schema.NullOr(RepositoryPath),
  kind: Schema.Literals(["added", "modified", "deleted", "renamed", "copied"]),
  coverage: Schema.Literals(["openapi", "component-root", "unmapped"]),
  capabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(100)),
  componentRoots: Schema.Array(RepositoryPath).check(Schema.isMaxLength(100)),
  unmappedPaths: Schema.Array(RepositoryPath).check(Schema.isMaxLength(2)),
}).annotate({
  identifier: "ChangedFileImpact",
  description: "Explicit component-root or semantic OpenAPI coverage for a changed file.",
});

export const CapabilityImpactV2 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/capability-impact/v2"]),
  analyzerVersion: Schema.Literals([
    "argus-component-roots/v1+argus-openapi/v1+libopenapi/v0.38.7",
  ]),
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
  status: Schema.Literals(["complete", "partial"]),
  semanticStatus: Schema.Literals(["complete", "partial"]),
  fileStatus: Schema.Literals(["complete", "partial"]),
  documents: Schema.Array(DocumentImpact).check(Schema.isMaxLength(50)),
  files: Schema.Array(FileImpact).check(Schema.isMaxLength(1000)),
  warnings: Schema.Array(Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(512))).check(
    Schema.isMaxLength(200),
  ),
  semanticWarnings: Schema.Array(
    Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(512)),
  ).check(Schema.isMaxLength(100)),
  fileWarnings: Schema.Array(
    Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(512)),
  ).check(Schema.isMaxLength(100)),
}).annotate({
  identifier: "CapabilityImpactV2",
  title: "Argus capability impact v2",
  description: "Semantic API and explicit changed-file mapping evidence at immutable revisions.",
});

export type CapabilityImpactV2 = typeof CapabilityImpactV2.Type;
