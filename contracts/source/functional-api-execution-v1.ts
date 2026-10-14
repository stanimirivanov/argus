import { Schema } from "effect";
import { ExecutionManifestV1APIVersion } from "./execution-manifest-v1.js";
import { DisplayName, LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const NonNegativeInteger = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0));
const AttemptId = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(127),
  Schema.isPattern(/^[A-Za-z0-9][A-Za-z0-9._:-]*$/u),
);
const SHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Version = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127));

const ManifestReference = Schema.Struct({
  apiVersion: ExecutionManifestV1APIVersion,
  sha256: SHA256,
});

const RequestedTest = Schema.Struct({
  suiteKey: LocalKey,
  testKey: LocalKey,
  name: DisplayName,
});

const Failure = Schema.Struct({
  code: LocalKey,
  message: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(2000)),
});

const NormalizedTestResult = Schema.Struct({
  suiteKey: LocalKey,
  testKey: LocalKey,
  outcome: Schema.Literals(["passed", "failed", "skipped", "error"]),
  durationMs: NonNegativeInteger.check(Schema.isLessThanOrEqualTo(86_400_000)),
  failure: Schema.NullOr(Failure),
});

const ArtifactReference = Schema.Struct({
  key: LocalKey,
  kind: LocalKey,
  uri: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(2048)),
  sha256: SHA256,
});

export const FunctionalAPIAdapterRequestV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/functional-api-adapter-request/v1"]),
  attemptId: AttemptId,
  manifest: ManifestReference,
  stage: Schema.Literals(["selected", "full-suite"]),
  testRepository: RepositoryReference,
  testRevision: Revision,
  adapter: LocalKey,
  tests: Schema.Array(RequestedTest).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
}).annotate({
  identifier: "FunctionalAPIAdapterRequestV1",
  title: "Argus functional API adapter request v1",
  description:
    "Exact functional API tests and immutable execution provenance supplied to a CI-local adapter.",
});

export const FunctionalAPIAdapterResultV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/functional-api-adapter-result/v1"]),
  attemptId: AttemptId,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  startedAt: Timestamp,
  completedAt: Timestamp,
  results: Schema.Array(NormalizedTestResult).check(
    Schema.isMinLength(1),
    Schema.isMaxLength(10000),
  ),
  artifacts: Schema.Array(ArtifactReference).check(Schema.isMaxLength(100)),
}).annotate({
  identifier: "FunctionalAPIAdapterResultV1",
  title: "Argus functional API adapter result v1",
  description:
    "Untrusted functional API adapter output before exact request correlation and normalization.",
});

export const ExecutionAttemptV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/execution-attempt/v1"]),
  attemptId: AttemptId,
  manifest: ManifestReference,
  stage: Schema.Literals(["selected", "full-suite"]),
  testRepository: RepositoryReference,
  testRevision: Revision,
  adapter: Schema.Struct({ id: LocalKey, version: Version }),
  startedAt: Timestamp,
  completedAt: Timestamp,
  outcome: Schema.Literals(["passed", "failed", "incomplete", "error"]),
  results: Schema.Array(NormalizedTestResult).check(
    Schema.isMinLength(1),
    Schema.isMaxLength(10000),
  ),
  artifacts: Schema.Array(ArtifactReference).check(Schema.isMaxLength(100)),
}).annotate({
  identifier: "ExecutionAttemptV1",
  title: "Argus execution attempt v1",
  description:
    "Normalized functional API attempt correlated to an exact execution manifest and test revision.",
});

export type FunctionalAPIAdapterRequestV1 = typeof FunctionalAPIAdapterRequestV1.Type;
export type FunctionalAPIAdapterResultV1 = typeof FunctionalAPIAdapterResultV1.Type;
export type ExecutionAttemptV1 = typeof ExecutionAttemptV1.Type;
