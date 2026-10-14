import { Schema } from "effect";

import { ExecutionManifestV1APIVersion } from "./execution-manifest-v1.js";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const NonNegativeInteger = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0));
const AttemptId = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(127),
  Schema.isPattern(/^[A-Za-z0-9][A-Za-z0-9._:-]*$/u),
);
const SHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const DurationMs = Schema.Int.check(
  Schema.isGreaterThanOrEqualTo(0),
  Schema.isLessThanOrEqualTo(864_000_000_000),
);
const DurationDeltaMs = Schema.Int.check(
  Schema.isGreaterThanOrEqualTo(-864_000_000_000),
  Schema.isLessThanOrEqualTo(864_000_000_000),
);

const FailureMiss = Schema.Struct({
  suiteKey: LocalKey,
  testKey: LocalKey,
  fullSuiteOutcome: Schema.Literals(["failed", "error"]),
  selectedOutcome: Schema.NullOr(Schema.Literals(["passed", "failed", "skipped", "error"])),
  reason: Schema.Literals(["not-selected", "not-reproduced"]),
});

export const SelectionShadowReportV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/selection-shadow-report/v1"]),
  manifest: Schema.Struct({
    apiVersion: ExecutionManifestV1APIVersion,
    sha256: SHA256,
  }),
  testRepository: RepositoryReference,
  testRevision: Revision,
  adapter: Schema.Struct({
    id: LocalKey,
    version: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  }),
  selectedAttemptId: AttemptId,
  fullSuiteAttemptId: AttemptId,
  selectedTestCount: NonNegativeInteger.check(Schema.isLessThanOrEqualTo(10000)),
  fullSuiteTestCount: NonNegativeInteger.check(Schema.isLessThanOrEqualTo(10000)),
  selectedDurationMs: DurationMs,
  fullSuiteDurationMs: DurationMs,
  durationReductionMs: DurationDeltaMs,
  selectedFailureCount: NonNegativeInteger.check(Schema.isLessThanOrEqualTo(10000)),
  fullSuiteFailureCount: NonNegativeInteger.check(Schema.isLessThanOrEqualTo(10000)),
  caughtFullSuiteFailureCount: NonNegativeInteger.check(Schema.isLessThanOrEqualTo(10000)),
  failureRecallBasisPoints: Schema.NullOr(
    NonNegativeInteger.check(Schema.isLessThanOrEqualTo(10000)),
  ),
  missedFailures: Schema.Array(FailureMiss).check(Schema.isMaxLength(10000)),
}).annotate({
  identifier: "SelectionShadowReportV1",
  title: "Argus selection shadow report v1",
  description:
    "Deterministic selected-versus-full-suite duration and failure-recall evidence for two explicit attempts.",
});

export type SelectionShadowReportV1 = typeof SelectionShadowReportV1.Type;
