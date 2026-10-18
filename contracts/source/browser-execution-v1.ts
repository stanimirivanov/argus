import { Schema } from "effect";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const AttemptId = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(127),
  Schema.isPattern(/^[A-Za-z0-9][A-Za-z0-9._:-]*$/u),
);
const Version = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127));
const Project = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(63),
  Schema.isPattern(/^[a-z][a-z0-9._-]*$/u),
);
const Test = Schema.Struct({
  suiteKey: LocalKey,
  testKey: LocalKey,
  name: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255)),
});
const Failure = Schema.Struct({
  code: LocalKey,
  message: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(2000)),
});
const TestResult = Schema.Struct({
  suiteKey: LocalKey,
  testKey: LocalKey,
  outcome: Schema.Literals(["passed", "failed", "skipped", "error"]),
  durationMs: Schema.Int.check(
    Schema.isGreaterThanOrEqualTo(0),
    Schema.isLessThanOrEqualTo(86400000),
  ),
  failure: Schema.NullOr(Failure),
});
const Artifact = Schema.Struct({
  key: LocalKey,
  suiteKey: LocalKey,
  testKey: LocalKey,
  kind: Schema.Literals(["trace", "screenshot", "accessibility"]),
  uri: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(2048)),
  sha256: Digest,
});
const Manifest = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-selection-manifest/v1"]),
  sha256: Digest,
});
export const BrowserAdapterRequestV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-adapter-request/v1"]),
  attemptId: AttemptId,
  manifest: Manifest,
  stage: Schema.Literals(["selected", "full-suite"]),
  testRepository: RepositoryReference,
  testRevision: Revision,
  adapter: LocalKey,
  project: Project,
  environment: Project,
  tests: Schema.Array(Test).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
}).annotate({ identifier: "BrowserAdapterRequestV1" });
export const BrowserAdapterResultV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-adapter-result/v1"]),
  attemptId: AttemptId,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  project: Project,
  environment: Project,
  startedAt: Timestamp,
  completedAt: Timestamp,
  results: Schema.Array(TestResult).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
  artifacts: Schema.Array(Artifact).check(Schema.isMaxLength(30000)),
}).annotate({ identifier: "BrowserAdapterResultV1" });
export const BrowserExecutionAttemptV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-execution-attempt/v1"]),
  attemptId: AttemptId,
  manifest: Manifest,
  stage: Schema.Literals(["selected", "full-suite"]),
  testRepository: RepositoryReference,
  testRevision: Revision,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  tests: Schema.Array(Test).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
  project: Project,
  environment: Project,
  startedAt: Timestamp,
  completedAt: Timestamp,
  outcome: Schema.Literals(["passed", "failed", "incomplete", "error"]),
  results: Schema.Array(TestResult).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
  artifacts: Schema.Array(Artifact).check(Schema.isMaxLength(30000)),
}).annotate({ identifier: "BrowserExecutionAttemptV1" });
