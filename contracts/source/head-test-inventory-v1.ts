import { Schema } from "effect";
import {
  DisplayName,
  LocalKey,
  RepositoryIdentity,
  RepositoryReference,
} from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const HeadTest = Schema.Struct({
  testRepository: RepositoryReference,
  revision: Revision,
  suiteKey: LocalKey,
  adapter: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  testKey: LocalKey,
  name: DisplayName,
  capabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(5000)),
  new: Schema.Boolean,
  modified: Schema.Boolean,
});

export const HeadTestInventoryV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/head-test-inventory/v1"]),
  sourceRepository: RepositoryIdentity,
  complete: Schema.Boolean,
  tests: Schema.Array(HeadTest).check(Schema.isMaxLength(10000)),
}).annotate({
  identifier: "HeadTestInventoryV1",
  description: "Complete functional API discovery at immutable test-repository revisions.",
});

export type HeadTestInventoryV1 = typeof HeadTestInventoryV1.Type;
