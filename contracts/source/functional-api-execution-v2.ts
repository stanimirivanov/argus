import { Schema } from "effect";
import {
  ExecutionAttemptV1,
  FunctionalAPIAdapterRequestV1,
} from "./functional-api-execution-v1.js";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const SHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Stage = Schema.Literals(["relevant-now", "remaining-later", "shadow-control"]);
const Mode = Schema.Literals(["shadow", "staged"]);
const Schedule = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/predictive-portfolio/v1"]),
  sha256: SHA256,
});
const Staging = Schema.Struct({
  schedule: Schedule,
  episodeId: LocalKey,
  groupKey: LocalKey,
  environment: LocalKey,
  mode: Mode,
});
const Binding = Schema.Struct({
  groupKey: LocalKey,
  testRepository: RepositoryReference,
  testRevision: Revision,
  adapter: LocalKey,
  environment: LocalKey,
});
export const FunctionalAPIExecutionBindingsV2 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/functional-api-execution-bindings/v2"]),
  groups: Schema.Array(Binding).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
});
const Candidate = Schema.Struct({
  id: SHA256,
  suiteKey: LocalKey,
  testKey: LocalKey,
  decision: Schema.Literals(["RUN_REQUIRED", "SKIP_FOR_NOW"]),
});
const IDs = Schema.Array(SHA256).check(Schema.isMaxLength(10000));
export const FunctionalAPIExecutionPlanV2 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/functional-api-execution-plan/v2"]),
  schedule: Schedule,
  episodeId: LocalKey,
  manifest: ExecutionAttemptV1.fields.manifest,
  mode: Mode,
  groups: Schema.Array(
    Schema.Struct({
      ...Binding.fields,
      candidates: Schema.Array(Candidate).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
      relevantNow: IDs,
      remainingLater: IDs,
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
});
export const FunctionalAPIAdapterRequestV2 = Schema.Struct({
  ...FunctionalAPIAdapterRequestV1.fields,
  apiVersion: Schema.Literals(["argus.dev/functional-api-adapter-request/v2"]),
  stage: Stage,
  staging: Staging,
});
export const ExecutionAttemptV2 = Schema.Struct({
  ...ExecutionAttemptV1.fields,
  apiVersion: Schema.Literals(["argus.dev/execution-attempt/v2"]),
  stage: Stage,
  staging: Staging,
});
