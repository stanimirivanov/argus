import { Schema } from "effect";

import { ExecutionManifestV1APIVersion } from "./execution-manifest-v1.js";
import { FunctionalAPIExecutionPlanV1APIVersion } from "./functional-api-execution-plan-v1.js";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const AttemptId = Schema.String.pipe(
  Schema.minLength(1),
  Schema.maxLength(127),
  Schema.pattern(/^[A-Za-z0-9][A-Za-z0-9._:-]*$/),
);
const SHA256 = Schema.String.pipe(Schema.pattern(/^[0-9a-f]{64}$/));
const Count = Schema.Int.pipe(Schema.greaterThanOrEqualTo(0), Schema.lessThanOrEqualTo(10000));
const DurationMs = Schema.Int.pipe(
  Schema.greaterThanOrEqualTo(0),
  Schema.lessThanOrEqualTo(864_000_000_000),
);
const DurationDeltaMs = Schema.Int.pipe(
  Schema.greaterThanOrEqualTo(-864_000_000_000),
  Schema.lessThanOrEqualTo(864_000_000_000),
);
const BasisPoints = Schema.NullOr(
  Schema.Int.pipe(Schema.greaterThanOrEqualTo(0), Schema.lessThanOrEqualTo(10000)),
);

const FailureMiss = Schema.Struct({
  suiteKey: LocalKey,
  testKey: LocalKey,
  fullSuiteOutcome: Schema.Literal("failed", "error"),
  selectedOutcome: Schema.NullOr(Schema.Literal("passed", "failed", "skipped", "error")),
  reason: Schema.Literal("not-selected", "not-reproduced"),
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
    version: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(127)),
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
  missedFailures: Schema.Array(FailureMiss).pipe(Schema.maxItems(10000)),
});

export const ExecutionPlanAttemptBindingsV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/execution-plan-attempt-bindings/v1"),
  groups: Schema.Array(GroupAttemptBinding).pipe(Schema.maxItems(10000)),
}).annotations({
  identifier: "ExecutionPlanAttemptBindingsV1",
  title: "Argus execution plan attempt bindings v1",
  description:
    "Explicit immutable attempt IDs for every selected and full-suite job in one execution plan.",
});

export const SelectionPlanShadowReportV1 = Schema.Struct({
  apiVersion: Schema.Literal("argus.dev/selection-plan-shadow-report/v1"),
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
  groups: Schema.Array(GroupReport).pipe(Schema.maxItems(10000)),
}).annotations({
  identifier: "SelectionPlanShadowReportV1",
  title: "Argus selection plan shadow report v1",
  description:
    "Aggregate duration and failure-recall evidence for every group in one immutable execution plan.",
});

export type ExecutionPlanAttemptBindingsV1 = typeof ExecutionPlanAttemptBindingsV1.Type;
export type SelectionPlanShadowReportV1 = typeof SelectionPlanShadowReportV1.Type;
