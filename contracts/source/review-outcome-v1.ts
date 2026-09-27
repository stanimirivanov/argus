import { Schema } from "effect";

import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const SHA256 = Schema.String.pipe(Schema.pattern(/^[0-9a-f]{64}$/));
const SourcePath = Schema.String.pipe(
  Schema.minLength(1),
  Schema.maxLength(4096),
  Schema.pattern(/^[^/\\][^\\]*$/),
);

const ReviewerFileEdit = Schema.Struct({
  path: SourcePath,
  previousPath: Schema.NullOr(SourcePath),
  kind: Schema.Literal("added", "modified", "deleted", "renamed", "copied"),
  additions: Schema.Int.pipe(Schema.greaterThanOrEqualTo(0)),
  deletions: Schema.Int.pipe(Schema.greaterThanOrEqualTo(0)),
  patch: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(65536)),
});

export const ReviewOutcomeV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/review-outcome/v1"),
  outcomeId: SHA256,
  reviewId: SHA256,
  proposalId: SHA256,
  validationId: SHA256,
  repository: RepositoryReference,
  pullRequest: Schema.Struct({
    number: Schema.Int.pipe(Schema.greaterThanOrEqualTo(1)),
    url: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(2048)),
  }),
  decision: Schema.Literal("accepted-as-proposed", "accepted-with-edits", "rejected"),
  reason: Schema.Struct({
    code: LocalKey,
    note: Schema.NullOr(Schema.String.pipe(Schema.minLength(1), Schema.maxLength(1000))),
  }),
  generatedRevision: Revision,
  finalRevision: Revision,
  closedAt: Schema.DateTimeUtc,
  mergedAt: Schema.NullOr(Schema.DateTimeUtc),
  observedAt: Schema.DateTimeUtc,
  reviewerEdits: Schema.Array(ReviewerFileEdit).pipe(Schema.maxItems(100)),
}).annotations({
  identifier: "ReviewOutcomeV1",
  title: "Argus review outcome v1",
  description:
    "Terminal provider-derived review disposition, explicit reason, and complete bounded diff after Argus' generated commit.",
});

export type ReviewOutcomeV1 = typeof ReviewOutcomeV1.Type;
