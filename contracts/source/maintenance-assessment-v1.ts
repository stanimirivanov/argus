import { Schema } from "effect";

const LocalKeyOrEmpty = Schema.String.check(
  Schema.isMaxLength(255),
  Schema.isPattern(/^$|^[a-z0-9][a-z0-9._-]*$/u),
);
const DigestOrEmpty = Schema.String.check(Schema.isPattern(/^$|^[0-9a-f]{64}$/u));
const AttemptId = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(127),
  Schema.isPattern(/^[A-Za-z0-9][A-Za-z0-9._:-]*$/u),
);

export const MaintenanceAssessmentV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/maintenance-assessment/v1"]),
  impactApiVersion: Schema.Literals([
    "argus.dev/capability-impact/v1",
    "argus.dev/capability-impact/v2",
  ]),
  impactAnalyzerVersion: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255)),
  sourceRepositoryId: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255)),
  baseRevision: Schema.String.check(Schema.isPattern(/^[0-9a-f]{40}$/u)),
  headRevision: Schema.String.check(Schema.isPattern(/^[0-9a-f]{40}$/u)),
  policyDigest: DigestOrEmpty,
  inventoryDigest: DigestOrEmpty,
  items: Schema.Array(
    Schema.Struct({
      repositoryId: Schema.String.check(Schema.isMaxLength(255)),
      suiteKey: LocalKeyOrEmpty,
      testKey: LocalKeyOrEmpty,
      capability: LocalKeyOrEmpty,
      state: Schema.Literals(["VALID", "INVALIDATED", "UNCOVERED", "UNCERTAIN"]),
      reason: Schema.Literals([
        "test-passed",
        "authoritative-endpoint-rename-and-test-failed",
        "no-mapped-test",
        "impact-incomplete",
        "conflicting-execution-evidence",
        "execution-evidence-missing",
        "failure-without-supported-rename",
        "non-assertion-result",
      ]),
      attemptIds: Schema.Array(AttemptId).check(Schema.isMaxLength(10000)),
    }),
  ).check(Schema.isMaxLength(15000)),
}).annotate({
  identifier: "MaintenanceAssessmentV1",
  description: "Conservative functional API maintenance state with immutable evidence identifiers.",
});

export type MaintenanceAssessmentV1 = typeof MaintenanceAssessmentV1.Type;
