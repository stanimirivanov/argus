import { Schema } from "effect";
import { LocalKey, RepositoryIdentity } from "./repository-descriptor-v1.js";

export const ReviewedTestIdentity = Schema.Struct({
  testRepository: RepositoryIdentity,
  suiteKey: LocalKey,
  testKey: LocalKey,
});

export const ReviewedSelectionPolicyV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/reviewed-selection-policy/v1"]),
  sourceRepository: RepositoryIdentity,
  mustRunCapabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(10000)),
  mustRunTests: Schema.Array(ReviewedTestIdentity).check(Schema.isMaxLength(10000)),
  pins: Schema.Array(ReviewedTestIdentity).check(Schema.isMaxLength(10000)),
  exclusions: Schema.Array(ReviewedTestIdentity).check(Schema.isMaxLength(10000)),
}).annotate({
  identifier: "ReviewedSelectionPolicyV1",
  description: "Owner-reviewed functional API selection rules scoped to one source repository.",
});

export type ReviewedSelectionPolicyV1 = typeof ReviewedSelectionPolicyV1.Type;
