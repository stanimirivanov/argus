import { Schema } from "effect";
import { RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const SHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Branch = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255));

export const AdaptationReviewV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/adaptation-review/v1"]),
  reviewId: SHA256,
  proposalId: SHA256,
  validationId: SHA256,
  repository: RepositoryReference,
  provider: Schema.Literals(["github"]),
  base: Schema.Struct({ branch: Branch, revision: Revision }),
  head: Schema.Struct({ branch: Branch, revision: Revision }),
  pullRequest: Schema.Struct({
    number: Schema.Int.check(Schema.isGreaterThanOrEqualTo(1)),
    url: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(2048)),
    draft: Schema.Literals([true]),
    state: Schema.Literals(["open"]),
  }),
  publishedAt: Timestamp,
}).annotate({
  identifier: "AdaptationReviewV1",
  title: "Argus adaptation review v1",
  description:
    "An open draft pull request published from one correlated proposal and successful validation proof.",
});

export type AdaptationReviewV1 = typeof AdaptationReviewV1.Type;
