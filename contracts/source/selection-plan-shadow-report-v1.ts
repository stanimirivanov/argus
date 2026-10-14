import { Schema } from "effect";

import { ExecutionManifestV1APIVersion } from "./execution-manifest-v1.js";
import { FunctionalAPIExecutionPlanV1APIVersion } from "./functional-api-execution-plan-v1.js";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const AttemptId = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(127),
  Schema.isPattern(/^[A-Za-z0-9][A-Za-z0-9._:-]*$/u),
);
const SHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Count = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0), Schema.isLessThanOrEqualTo(10000));
const DurationMs = Schema.Int.check(
  Schema.isGreaterThanOrEqualTo(0),
  Schema.isLessThanOrEqualTo(864_000_000_000),
);
const DurationDeltaMs = Schema.Int.check(
  Schema.isGreaterThanOrEqualTo(-864_000_000_000),
  Schema.isLessThanOrEqualTo(864_000_000_000),
);
const BasisPoints = Schema.NullOr(
  Schema.Int.check(Schema.isGreaterThanOrEqualTo(0), Schema.isLessThanOrEqualTo(10000)),
);

const FailureMiss = Schema.Struct({
  suiteKey: LocalKey,
  testKey: LocalKey,
  fullSuiteOutcome: Schema.Literals(["failed", "error"]),
  selectedOutcome: Schema.NullOr(Schema.Literals(["passed", "failed", "skipped", "error"])),
  reason: Schema.Literals(["not-selected", "not-reproduced"]),
});

const GroupAttemptBinding = Schema.Struct({
  groupKey: LocalKey,
  selectedAttemptId: Schema.NullOr(AttemptId),
  fullSuiteAttemptId: AttemptId,
});

const GroupReport = Schema.Struct({
  groupKey: LocalKey,
  testRepository: RepositoryReference,
  testRevision: Revision,
  adapter: Schema.Struct({
    id: LocalKey,
    version: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  }),
  selectedAttemptId: Schema.NullOr(AttemptId),
  fullSuiteAttemptId: AttemptId,
  selectedTestCount: Count,
  fullSuiteTestCount: Count,
  selectedDurationMs: DurationMs,
  fullSuiteDurationMs: DurationMs,
  durationReductionMs: DurationDeltaMs,
  selectedFailureCount: Count,
  fullSuiteFailureCount: Count,
  caughtFullSuiteFailureCount: Count,
  failureRecallBasisPoints: BasisPoints,
  missedFailures: Schema.Array(FailureMiss).check(Schema.isMaxLength(10000)),
});

export const ExecutionPlanAttemptBindingsV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/execution-plan-attempt-bindings/v1"]),
  groups: Schema.Array(GroupAttemptBinding).check(Schema.isMaxLength(10000)),
}).annotate({
  identifier: "ExecutionPlanAttemptBindingsV1",
  title: "Argus execution plan attempt bindings v1",
  description:
    "Explicit immutable attempt IDs for every selected and full-suite job in one execution plan.",
});

export const SelectionPlanShadowReportV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/selection-plan-shadow-report/v1"]),
  plan: Schema.Struct({
    apiVersion: FunctionalAPIExecutionPlanV1APIVersion,
    sha256: SHA256,
  }),
  manifest: Schema.Struct({
    apiVersion: ExecutionManifestV1APIVersion,
    sha256: SHA256,
  }),
  groupCount: Count,
  groupsWithSelection: Count,
  fullOnlyGroupCount: Count,
  selectedTestCount: Count,
  fullSuiteTestCount: Count,
  selectedDurationMs: DurationMs,
  fullSuiteDurationMs: DurationMs,
  durationReductionMs: DurationDeltaMs,
  selectedFailureCount: Count,
  fullSuiteFailureCount: Count,
  caughtFullSuiteFailureCount: Count,
  failureRecallBasisPoints: BasisPoints,
  groups: Schema.Array(GroupReport).check(Schema.isMaxLength(10000)),
}).annotate({
  identifier: "SelectionPlanShadowReportV1",
  title: "Argus selection plan shadow report v1",
  description:
    "Aggregate duration and failure-recall evidence for every group in one immutable execution plan.",
});

export type ExecutionPlanAttemptBindingsV1 = typeof ExecutionPlanAttemptBindingsV1.Type;
export type SelectionPlanShadowReportV1 = typeof SelectionPlanShadowReportV1.Type;
