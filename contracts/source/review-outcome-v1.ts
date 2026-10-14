import { Schema } from "effect";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const SHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const SourcePath = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(4096),
  Schema.isPattern(/^[^/\\][^\\]*$/u),
);

const ReviewerFileEdit = Schema.Struct({
  path: SourcePath,
  previousPath: Schema.NullOr(SourcePath),
  kind: Schema.Literals(["added", "modified", "deleted", "renamed", "copied"]),
  additions: Schema.Int.check(Schema.isGreaterThanOrEqualTo(0)),
  deletions: Schema.Int.check(Schema.isGreaterThanOrEqualTo(0)),
  patch: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(65536)),
});

export const ReviewOutcomeV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/review-outcome/v1"]),
  outcomeId: SHA256,
  reviewId: SHA256,
  proposalId: SHA256,
  validationId: SHA256,
  repository: RepositoryReference,
  pullRequest: Schema.Struct({
    number: Schema.Int.check(Schema.isGreaterThanOrEqualTo(1)),
    url: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(2048)),
  }),
  decision: Schema.Literals(["accepted-as-proposed", "accepted-with-edits", "rejected"]),
  reason: Schema.Struct({
    code: LocalKey,
    note: Schema.NullOr(Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(1000))),
  }),
  generatedRevision: Revision,
  finalRevision: Revision,
  closedAt: Timestamp,
  mergedAt: Schema.NullOr(Timestamp),
  observedAt: Timestamp,
  reviewerEdits: Schema.Array(ReviewerFileEdit).check(Schema.isMaxLength(100)),
}).annotate({
  identifier: "ReviewOutcomeV1",
  title: "Argus review outcome v1",
  description:
    "Terminal provider-derived review disposition, explicit reason, and complete bounded diff after Argus' generated commit.",
});

export type ReviewOutcomeV1 = typeof ReviewOutcomeV1.Type;
