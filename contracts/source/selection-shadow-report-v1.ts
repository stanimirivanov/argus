import { Schema } from "effect";

import { ExecutionManifestV1APIVersion } from "./execution-manifest-v1.js";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const NonNegativeInteger = Schema.Int.pipe(Schema.greaterThanOrEqualTo(0));
const AttemptId = Schema.String.pipe(
  Schema.minLength(1),
  Schema.maxLength(127),
  Schema.pattern(/^[A-Za-z0-9][A-Za-z0-9._:-]*$/),
);
const SHA256 = Schema.String.pipe(Schema.pattern(/^[0-9a-f]{64}$/));
const DurationMs = Schema.Int.pipe(
  Schema.greaterThanOrEqualTo(0),
  Schema.lessThanOrEqualTo(864_000_000_000),
);
const DurationDeltaMs = Schema.Int.pipe(
  Schema.greaterThanOrEqualTo(-864_000_000_000),
  Schema.lessThanOrEqualTo(864_000_000_000),
);

const FailureMiss = Schema.Struct({
  suiteKey: LocalKey,
  testKey: LocalKey,
  fullSuiteOutcome: Schema.Literal("failed", "error"),
  selectedOutcome: Schema.NullOr(Schema.Literal("passed", "failed", "skipped", "error")),
  reason: Schema.Literal("not-selected", "not-reproduced"),
});

export const SelectionShadowReportV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/selection-shadow-report/v1"),
  manifest: Schema.Struct({
    apiVersion: ExecutionManifestV1APIVersion,
    sha256: SHA256,
  }),
  testRepository: RepositoryReference,
  testRevision: Revision,
  adapter: Schema.Struct({
    id: LocalKey,
    version: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(127)),
  }),
  selectedAttemptId: AttemptId,
  fullSuiteAttemptId: AttemptId,
  selectedTestCount: NonNegativeInteger.pipe(Schema.lessThanOrEqualTo(10000)),
  fullSuiteTestCount: NonNegativeInteger.pipe(Schema.lessThanOrEqualTo(10000)),
  selectedDurationMs: DurationMs,
  fullSuiteDurationMs: DurationMs,
  durationReductionMs: DurationDeltaMs,
  selectedFailureCount: NonNegativeInteger.pipe(Schema.lessThanOrEqualTo(10000)),
  fullSuiteFailureCount: NonNegativeInteger.pipe(Schema.lessThanOrEqualTo(10000)),
  caughtFullSuiteFailureCount: NonNegativeInteger.pipe(Schema.lessThanOrEqualTo(10000)),
  failureRecallBasisPoints: Schema.NullOr(NonNegativeInteger.pipe(Schema.lessThanOrEqualTo(10000))),
  missedFailures: Schema.Array(FailureMiss).pipe(Schema.maxItems(10000)),
}).annotations({
  identifier: "SelectionShadowReportV1",
  title: "Argus selection shadow report v1",
  description:
    "Deterministic selected-versus-full-suite duration and failure-recall evidence for two explicit attempts.",
});

export type SelectionShadowReportV1 = typeof SelectionShadowReportV1.Type;
