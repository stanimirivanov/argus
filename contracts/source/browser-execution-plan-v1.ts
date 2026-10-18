import { Schema } from "effect";

import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const SHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Project = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(63),
  Schema.isPattern(/^[a-z][a-z0-9._-]*$/u),
);
const AttemptId = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(127),
  Schema.isPattern(/^[A-Za-z0-9][A-Za-z0-9._:-]*$/u),
);
const Count = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0), Schema.isLessThanOrEqualTo(10000));
const PositiveCount = Schema.Int.check(
  Schema.isGreaterThanOrEqualTo(1),
  Schema.isLessThanOrEqualTo(10000),
);
const Duration = Schema.Int.check(
  Schema.isGreaterThanOrEqualTo(0),
  Schema.isLessThanOrEqualTo(864_000_000_000),
);
const Delta = Schema.Int.check(
  Schema.isGreaterThanOrEqualTo(-864_000_000_000),
  Schema.isLessThanOrEqualTo(864_000_000_000),
);
const BasisPoints = Schema.NullOr(
  Schema.Int.check(Schema.isGreaterThanOrEqualTo(0), Schema.isLessThanOrEqualTo(10000)),
);
const Manifest = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-selection-manifest/v1"]),
  sha256: SHA256,
});
const GroupFields = {
  groupKey: LocalKey,
  testRepository: RepositoryReference,
  testRevision: Revision,
  adapter: LocalKey,
  project: Project,
  environment: Project,
};
const Group = Schema.Struct(GroupFields);

export const BrowserExecutionBindingsV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-execution-bindings/v1"]),
  groups: Schema.Array(Group).check(Schema.isMaxLength(10000)),
}).annotate({
  identifier: "BrowserExecutionBindingsV1",
  description: "Reviewed browser checkout, project, and environment groups.",
});

export const BrowserExecutionPlanV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-execution-plan/v1"]),
  manifest: Manifest,
  jobs: Schema.Array(
    Schema.Struct({
      ...GroupFields,
      stage: Schema.Literals(["selected", "full-suite"]),
      testCount: PositiveCount,
    }),
  ).check(Schema.isMaxLength(20000)),
}).annotate({
  identifier: "BrowserExecutionPlanV1",
  description: "Deterministic browser CI jobs with mandatory full-suite controls.",
});

export const BrowserPlanAttemptBindingsV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-plan-attempt-bindings/v1"]),
  groups: Schema.Array(
    Schema.Struct({
      groupKey: LocalKey,
      selectedAttemptId: Schema.NullOr(AttemptId),
      fullSuiteAttemptId: AttemptId,
    }),
  ).check(Schema.isMaxLength(10000)),
}).annotate({
  identifier: "BrowserPlanAttemptBindingsV1",
  description: "Explicit immutable browser attempt IDs for every planned group.",
});

const FailureMiss = Schema.Struct({
  suiteKey: LocalKey,
  testKey: LocalKey,
  fullSuiteOutcome: Schema.Literals(["failed", "error"]),
  selectedOutcome: Schema.NullOr(Schema.Literals(["passed", "failed", "skipped", "error"])),
  reason: Schema.Literals(["not-selected", "not-reproduced"]),
});
const Metrics = {
  selectedTestCount: Count,
  fullSuiteTestCount: Count,
  selectedDurationMs: Duration,
  fullSuiteDurationMs: Duration,
  durationReductionMs: Delta,
  selectedFailureCount: Count,
  fullSuiteFailureCount: Count,
  caughtFullSuiteFailureCount: Count,
  failureRecallBasisPoints: BasisPoints,
};
export const BrowserPlanShadowReportV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-plan-shadow-report/v1"]),
  plan: Schema.Struct({
    apiVersion: Schema.Literals(["argus.dev/browser-execution-plan/v1"]),
    sha256: SHA256,
  }),
  manifest: Manifest,
  groupCount: Count,
  groupsWithSelection: Count,
  fullOnlyGroupCount: Count,
  ...Metrics,
  groups: Schema.Array(
    Schema.Struct({
      ...GroupFields,
      adapterVersion: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
      selectedAttemptId: Schema.NullOr(AttemptId),
      fullSuiteAttemptId: AttemptId,
      ...Metrics,
      missedFailures: Schema.Array(FailureMiss).check(Schema.isMaxLength(10000)),
    }),
  ).check(Schema.isMaxLength(10000)),
}).annotate({
  identifier: "BrowserPlanShadowReportV1",
  description: "Complete browser selection miss and failure recall evidence.",
});
