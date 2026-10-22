import { Schema } from "effect";
import { BrowserAdapterRequestV1, BrowserExecutionAttemptV1 } from "./browser-execution-v1.js";
import { FunctionalAPIAdapterRequestV2 } from "./functional-api-execution-v2.js";
import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";

const SHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const Staging = Schema.Struct({
  ...FunctionalAPIAdapterRequestV2.fields.staging.fields,
  discoverySha256: SHA256,
});
const Stage = FunctionalAPIAdapterRequestV2.fields.stage;
const Candidates = BrowserAdapterRequestV1.fields.tests;
export const BrowserAdapterRequestV2 = Schema.Struct({
  ...BrowserAdapterRequestV1.fields,
  apiVersion: Schema.Literals(["argus.dev/browser-adapter-request/v2"]),
  stage: Stage,
  staging: Staging,
  candidates: Candidates,
});
export const BrowserExecutionAttemptV2 = Schema.Struct({
  ...BrowserExecutionAttemptV1.fields,
  apiVersion: Schema.Literals(["argus.dev/browser-execution-attempt/v2"]),
  stage: Stage,
  staging: Staging,
  candidates: Candidates,
});
const IDs = Schema.Array(SHA256).check(Schema.isMaxLength(10000));
export const BrowserExecutionPlanV2 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-execution-plan/v2"]),
  schedule: Staging.fields.schedule,
  episodeId: LocalKey,
  discoverySha256: SHA256,
  manifest: BrowserAdapterRequestV1.fields.manifest,
  mode: Staging.fields.mode,
  groups: Schema.Array(
    Schema.Struct({
      groupKey: LocalKey,
      testRepository: RepositoryReference,
      testRevision: Revision,
      adapter: LocalKey,
      project: LocalKey,
      environment: LocalKey,
      candidates: Schema.Array(
        Schema.Struct({
          id: SHA256,
          suiteKey: LocalKey,
          testKey: LocalKey,
          decision: Schema.Literals(["RUN_REQUIRED", "SKIP_FOR_NOW"]),
        }),
      ).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
      relevantNow: IDs,
      remainingLater: IDs,
    }),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(10000)),
});
