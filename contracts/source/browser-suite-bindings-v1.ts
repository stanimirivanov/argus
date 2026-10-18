import { Schema } from "effect";

import { LocalKey, RepositoryIdentity } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

/** Reviewed CI checkout pins used to compare discovery with intended execution. */
export const BrowserSuiteBindingsV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-suite-bindings/v1"]),
  sourceRepository: RepositoryIdentity,
  suites: Schema.Array(
    Schema.Struct({
      testRepository: RepositoryIdentity,
      suiteKey: LocalKey,
      revision: Revision,
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
}).annotate({
  identifier: "BrowserSuiteBindingsV1",
  description:
    "Explicit immutable browser test checkout revisions selected by reviewed CI configuration.",
});

export type BrowserSuiteBindingsV1 = typeof BrowserSuiteBindingsV1.Type;
