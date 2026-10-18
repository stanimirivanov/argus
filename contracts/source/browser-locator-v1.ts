import { Schema } from "effect";
import { LocalKey } from "./repository-descriptor-v1.js";
import { Timestamp } from "./timestamp.js";

const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
const AttemptID = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127));
const SourcePath = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(4096),
  Schema.isPattern(/^[^/\\][^\\]*$/u),
);

export const BrowserStructureV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-structure/v1"]),
  capturedAt: Timestamp,
  url: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(2048)),
  complete: Schema.Boolean,
  nodes: Schema.Array(
    Schema.Struct({
      role: Schema.String.check(Schema.isMaxLength(100)),
      name: Schema.String.check(Schema.isMaxLength(200)),
      testId: Schema.String.check(Schema.isMaxLength(200)),
      visible: Schema.Boolean,
    }),
  ).check(Schema.isMaxLength(10000)),
  ariaSnapshot: Schema.String.check(Schema.isMaxLength(1048576)),
}).annotate({ identifier: "BrowserStructureV1" });

const Target = Schema.Struct({
  role: Schema.String.check(Schema.isMaxLength(100)),
  name: Schema.String.check(Schema.isMaxLength(200)),
  testId: Schema.String.check(Schema.isMaxLength(200)),
  visible: Schema.Boolean,
});

const TextEdit = Schema.Struct({
  path: SourcePath,
  beforeSha256: Digest,
  startByte: Schema.Int.check(Schema.isGreaterThanOrEqualTo(0)),
  endByte: Schema.Int.check(Schema.isGreaterThan(0)),
  original: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(500)),
  replacement: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(500)),
  semanticRole: Schema.Literals(["browser-locator"]),
});

export const BrowserLocatorProposalV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-locator-proposal/v1"]),
  policyVersion: Schema.Literals(["argus.dev/adaptation-policy/browser-stable-locator/v1"]),
  proposalId: Digest,
  baselineAttemptId: AttemptID,
  failedAttemptId: AttemptID,
  baselineSnapshotSha256: Digest,
  failedSnapshotSha256: Digest,
  suiteKey: LocalKey,
  testKey: LocalKey,
  originalTarget: Target,
  currentTarget: Target,
  edit: TextEdit,
}).annotate({ identifier: "BrowserLocatorProposalV1" });

export const BrowserLocatorDiagnosisV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-locator-diagnosis/v1"]),
  reasonCode: Schema.Literals([
    "candidate",
    "non-locator-failure",
    "incomplete-structure",
    "positional-locator",
    "unproved-original-target",
    "ambiguous-or-hidden-target",
    "unsupported-replacement",
    "failure-source-mismatch",
    "ambiguous-or-unsupported-source",
    "cross-flow-source",
    "cross-flow-navigation",
  ]),
  baselineAttemptId: AttemptID,
  failedAttemptId: AttemptID,
  suiteKey: LocalKey,
  testKey: LocalKey,
  proposal: Schema.NullOr(BrowserLocatorProposalV1),
}).annotate({ identifier: "BrowserLocatorDiagnosisV1" });
