import { Schema } from "effect";
import { ChangeReference } from "./execution-manifest-v1.js";
import { PerformanceReference } from "./performance-selection-v1.js";
import { Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const Text = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(256),
  Schema.isPattern(/\S/u),
);
const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Identity = Schema.Struct({
  issuer: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(512)),
  id: Text,
});
const RevisionNumber = Schema.Int.check(
  Schema.isGreaterThanOrEqualTo(0),
  Schema.isLessThanOrEqualTo(Number.MAX_SAFE_INTEGER),
);
const Reasons = Schema.Array(
  Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(1024)),
).check(Schema.isMaxLength(1000));
const Passing = Schema.Struct({ status: Schema.Literals(["PASS"]), reasons: Reasons });
const Unresolved = Schema.Struct({
  status: Schema.Literals(["INCONCLUSIVE", "NOT_EVALUATED"]),
  reasons: Reasons.check(Schema.isMinLength(1)),
});
const Comparison = Schema.Union([
  Passing,
  Schema.Struct({
    status: Schema.Literals(["FAIL"]),
    reasons: Reasons.check(Schema.isMinLength(1)),
  }),
  Unresolved,
]);
const Selector = {
  ruleId: Text,
  metric: Text.check(Schema.isPattern(/^[a-z][a-z0-9_.]*$/u)),
  statistic: Schema.Literals(["mean", "median", "p90", "p95", "p99", "min", "max"]),
  unit: Text,
};
// Match the verified native report: failed quality cannot yield decisive comparisons.
const Rule = Schema.Union([
  Schema.Struct({ ...Selector, quality: Passing, slo: Comparison, regression: Comparison }),
  Schema.Struct({
    ...Selector,
    quality: Schema.Struct({
      status: Schema.Literals(["INVALID", "UNSTABLE", "INCONCLUSIVE", "NOT_EVALUATED"]),
      reasons: Reasons.check(Schema.isMinLength(1)),
    }),
    slo: Unresolved,
    regression: Unresolved,
  }),
]);
const Decision = Schema.Struct({
  eventId: Text,
  evidenceFingerprint: Digest,
  reportSha256: Digest,
  publishedAt: Timestamp,
  late: Schema.Boolean,
  rules: Schema.Array(Rule).check(Schema.isMinLength(1), Schema.isMaxLength(1000)),
});

/** Allowlisted metadata for one admitted request, never approval or an overall verdict.
 * Decode with onExcessProperty:error. Producers must enforce the semantic invariants
 * and 8 MiB UTF-8 output limit documented in performance-decision-projection.md.
 */
export const PerformanceDecisionProjectionV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/performance-decision-projection/v1"]),
  scope: Schema.Struct({
    issuer: Identity.fields.issuer,
    principalIssuer: Identity.fields.issuer,
    principalId: Text,
  }),
  provenance: Schema.Struct({
    change: ChangeReference,
    impactSha256: Digest,
    selectionSha256: Digest,
    mappingPolicySha256: Digest,
    catalogSha256: Digest,
    reviewSha256: Digest,
    intentFingerprint: Digest,
  }),
  request: Schema.Struct({
    reference: Identity,
    attempt: Identity,
    eventId: Text,
    payloadSha256: Digest,
    requestedAt: Timestamp,
    deadline: Timestamp,
  }),
  target: Schema.Struct({
    service: PerformanceReference,
    capability: PerformanceReference,
    deployment: PerformanceReference,
    source: Revision,
    imageSha256: Digest,
    workload: PerformanceReference,
    profile: PerformanceReference,
    environment: PerformanceReference,
    policy: PerformanceReference,
    baseline: Schema.NullOr(PerformanceReference),
  }),
  run: Identity,
  lifecycle: Schema.NullOr(
    Schema.Struct({
      eventId: Text,
      evidenceFingerprint: Digest,
      revision: RevisionNumber,
      state: Schema.Literals([
        "ACCEPTED",
        "RUNNING",
        "CANCELLING",
        "COMPLETED",
        "FAILED",
        "CANCELLED",
      ]),
      observedAt: Timestamp,
    }),
  ),
  analysis: Schema.Union([
    Schema.Struct({
      conflict: Schema.Literals([false]),
      decisions: Schema.Array(Decision).check(Schema.isMaxLength(1000)),
    }),
    Schema.Struct({
      conflict: Schema.Literals([true]),
      decisions: Schema.Array(Decision).check(Schema.isMinLength(2), Schema.isMaxLength(1000)),
    }),
  ]),
});
