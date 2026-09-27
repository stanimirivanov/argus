import { Schema } from "effect";

import { RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const SHA256 = Schema.String.pipe(Schema.pattern(/^[0-9a-f]{64}$/));
const Branch = Schema.String.pipe(Schema.minLength(1), Schema.maxLength(255));

export const AdaptationReviewV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/adaptation-review/v1"),
  reviewId: SHA256,
  proposalId: SHA256,
  validationId: SHA256,
  repository: RepositoryReference,
  provider: Schema.Literal("github"),
  base: Schema.Struct({ branch: Branch, revision: Revision }),
  head: Schema.Struct({ branch: Branch, revision: Revision }),
  pullRequest: Schema.Struct({
    number: Schema.Int.pipe(Schema.greaterThanOrEqualTo(1)),
    url: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(2048)),
    draft: Schema.Literal(true),
    state: Schema.Literal("open"),
  }),
  publishedAt: Schema.DateTimeUtc,
}).annotations({
  identifier: "AdaptationReviewV1",
  title: "Argus adaptation review v1",
  description:
    "An open draft pull request published from one correlated proposal and successful validation proof.",
});

export type AdaptationReviewV1 = typeof AdaptationReviewV1.Type;
