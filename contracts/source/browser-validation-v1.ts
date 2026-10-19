import { Schema } from "effect";
import { BrowserExecutionAttemptV1 } from "./browser-execution-v1.js";
import { BrowserLocatorProposalV1 } from "./browser-locator-v1.js";

const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));

export const BrowserLocatorValidationV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-locator-validation/v1"]),
  policyVersion: Schema.Literals(["argus.dev/validation-policy/browser-stable-locator/v1"]),
  validationId: Digest,
  status: Schema.Literals(["validated", "rejected"]),
  reason: Schema.optional(
    Schema.Literals(["original-passed", "candidate-locator-failed", "negative-control-passed"]),
  ),
  proposal: BrowserLocatorProposalV1,
  baseline: BrowserExecutionAttemptV1,
  failed: BrowserExecutionAttemptV1,
  source: Schema.Struct({
    path: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(4096)),
    originalSha256: Digest,
    candidateSha256: Digest,
    negativeSha256: Digest,
    restoredSha256: Digest,
    negativeLocator: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(500)),
  }),
  runs: Schema.Array(
    Schema.Struct({
      phase: Schema.Literals(["original", "candidate", "negative-control"]),
      repeat: Schema.Literals([1, 2]),
      sourceSha256: Digest,
      attempt: BrowserExecutionAttemptV1,
    }),
  ).check(Schema.isMinLength(2), Schema.isMaxLength(6)),
}).annotate({ identifier: "BrowserLocatorValidationV1" });
