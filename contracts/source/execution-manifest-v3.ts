import { Schema } from "effect";

import { ExecutionManifestV2 } from "./execution-manifest-v2.js";

// v3 retains the browser decision shape while replacing the evidence producer
// and policy. A v2 consumer must reject it rather than reinterpret its impact.
export const ExecutionManifestV3 = Schema.Struct({
  ...ExecutionManifestV2.fields,
  apiVersion: Schema.Literal("argus.dev/execution-manifest/v3"),
  policyVersion: Schema.Literal("argus.dev/selection-policy/functional-ui-component/v1"),
  impact: Schema.Struct({
    apiVersion: Schema.Literal("argus.dev/component-root-impact/v1"),
    analyzerVersion: Schema.Literal("argus-component-roots/v1"),
  }),
}).annotations({
  identifier: "ExecutionManifestV3",
  title: "Argus browser component selection manifest v3",
  description:
    "Deterministic functional UI decisions from complete changed-file evidence and base-catalog component roots.",
});

export type ExecutionManifestV3 = typeof ExecutionManifestV3.Type;
