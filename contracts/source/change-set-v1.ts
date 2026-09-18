import { Schema } from "effect";

import { RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const PositiveInteger = Schema.Int.pipe(Schema.greaterThanOrEqualTo(1));
const NonNegativeInteger = Schema.Int.pipe(Schema.greaterThanOrEqualTo(0));
const RepositoryPath = Schema.String.pipe(
  Schema.minLength(1),
  Schema.maxLength(4096),
  Schema.pattern(/^[^/\\][^\\]*$/),
);
const DeliveryID = Schema.String.pipe(Schema.minLength(1), Schema.maxLength(255));
const Patch = Schema.String.pipe(Schema.maxLength(65536));

export const FileChangeKind = Schema.Literal("added", "modified", "deleted", "renamed", "copied");
export const PatchStatus = Schema.Literal(
  "complete",
  "unavailable",
  "truncated",
  "budget-exhausted",
);

export const ChangeFile = Schema.Struct({
  path: RepositoryPath,
  previousPath: Schema.NullOr(RepositoryPath),
  kind: FileChangeKind,
  additions: NonNegativeInteger,
  deletions: NonNegativeInteger,
  patch: Schema.NullOr(Patch),
  patchStatus: PatchStatus,
}).annotations({
  identifier: "ChangeFile",
  description:
    "One normalized repository-relative file change. Patch status distinguishes missing evidence from bounded evidence.",
});

export const ChangeSetV1APIVersion = Schema.Literal("argus.dev/change-set/v1");

export const ChangeSetV1 = Schema.Struct({
  apiVersion: ChangeSetV1APIVersion,
  sourceRepository: RepositoryReference,
  pullRequest: Schema.Struct({
    number: PositiveInteger,
  }),
  baseRevision: Revision,
  headRevision: Revision,
  observedAt: Schema.DateTimeUtc,
  trigger: Schema.Struct({
    provider: Schema.Literal("github"),
    deliveryId: DeliveryID,
    event: Schema.Literal("pull_request"),
    action: Schema.Literal("opened", "reopened", "synchronize", "ready_for_review"),
  }),
  files: Schema.Array(ChangeFile).pipe(Schema.maxItems(1000)),
  filesTruncated: Schema.Boolean,
}).annotations({
  identifier: "ChangeSetV1",
  title: "Argus change set v1",
  description:
    "A reproducible, bounded pull-request change observation at immutable base and head revisions.",
});

export type ChangeSetV1 = typeof ChangeSetV1.Type;
