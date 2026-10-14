import { Schema } from "effect";
import { RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const PositiveInteger = Schema.Int.check(Schema.isGreaterThanOrEqualTo(1));
const NonNegativeInteger = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0));
const RepositoryPath = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(4096),
  Schema.isPattern(/^[^/\\][^\\]*$/u),
);
const DeliveryID = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255));
const Patch = Schema.String.check(Schema.isMaxLength(65536));

export const FileChangeKind = Schema.Literals([
  "added",
  "modified",
  "deleted",
  "renamed",
  "copied",
]);
export const PatchStatus = Schema.Literals([
  "complete",
  "unavailable",
  "truncated",
  "budget-exhausted",
]);

export const ChangeFile = Schema.Struct({
  path: RepositoryPath,
  previousPath: Schema.NullOr(RepositoryPath),
  kind: FileChangeKind,
  additions: NonNegativeInteger,
  deletions: NonNegativeInteger,
  patch: Schema.NullOr(Patch),
  patchStatus: PatchStatus,
}).annotate({
  identifier: "ChangeFile",
  description:
    "One normalized repository-relative file change. Patch status distinguishes missing evidence from bounded evidence.",
});

export const ChangeSetV1APIVersion = Schema.Literals(["argus.dev/change-set/v1"]);

export const ChangeSetV1 = Schema.Struct({
  apiVersion: ChangeSetV1APIVersion,
  sourceRepository: RepositoryReference,
  pullRequest: Schema.Struct({
    number: PositiveInteger,
  }),
  baseRevision: Revision,
  headRevision: Revision,
  observedAt: Timestamp,
  trigger: Schema.Struct({
    provider: Schema.Literals(["github"]),
    deliveryId: DeliveryID,
    event: Schema.Literals(["pull_request"]),
    action: Schema.Literals(["opened", "reopened", "synchronize", "ready_for_review"]),
  }),
  files: Schema.Array(ChangeFile).check(Schema.isMaxLength(1000)),
  filesTruncated: Schema.Boolean,
}).annotate({
  identifier: "ChangeSetV1",
  title: "Argus change set v1",
  description:
    "A reproducible, bounded pull-request change observation at immutable base and head revisions.",
});

export type ChangeSetV1 = typeof ChangeSetV1.Type;
