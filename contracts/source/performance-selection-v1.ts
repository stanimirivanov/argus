import { Schema } from "effect";
import { ChangeReference } from "./execution-manifest-v1.js";
import { LocalKey, RepositoryIdentity } from "./repository-descriptor-v1.js";
import { Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const Text = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(256));
const Issuer = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(512));
const Digest = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));
export const PerformanceReference = Schema.Struct({
  issuer: Issuer,
  id: Text,
  revision: Text,
  sha256: Digest,
});
const Identity = Schema.Struct({ issuer: Issuer, id: Text });
const Mapping = Schema.Struct({
  id: Text,
  capability: LocalKey,
  workload: PerformanceReference,
  profile: PerformanceReference,
  environment: PerformanceReference,
  policy: PerformanceReference,
  baseline: Schema.NullOr(PerformanceReference),
});
const Deployment = Schema.Struct({
  environment: PerformanceReference,
  reference: Schema.NullOr(PerformanceReference),
  service: Schema.NullOr(Identity),
  imageSha256: Schema.NullOr(Digest),
  sourceRevision: Revision,
});
export const PerformanceMappingPolicyV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/performance-mapping-policy/v1"]),
  repository: RepositoryIdentity,
  headRevision: Revision,
  impactSha256: Digest,
  catalogSha256: Digest,
  reviewId: Text,
  reviewedBy: Text,
  reviewedAt: Timestamp,
  expiresAt: Timestamp,
  headCapabilities: Schema.Array(LocalKey).check(Schema.isMaxLength(5000)),
  allowedProfiles: Schema.Array(PerformanceReference).check(Schema.isMaxLength(1000)),
  deployments: Schema.Array(Deployment).check(Schema.isMaxLength(1000)),
  mappings: Schema.Array(Mapping).check(Schema.isMaxLength(1000)),
});

const Candidate = Schema.Struct({
  mappingId: Text,
  capability: LocalKey,
  outcome: Schema.Literals(["candidate"]),
  reason: Schema.Literals(["owner-approval-required"]),
  target: Schema.Struct({ ...Mapping.fields, baseline: PerformanceReference }),
  ownerRoute: PerformanceReference,
  deployment: Schema.Struct({
    ...Deployment.fields,
    reference: PerformanceReference,
    service: Identity,
    imageSha256: Digest,
  }),
});
const Omitted = Schema.Struct({
  mappingId: Text,
  capability: LocalKey,
  outcome: Schema.Literals(["omitted"]),
  reason: Schema.Literals(["capability-unaffected"]),
  target: Mapping,
  ownerRoute: Schema.Null,
  deployment: Schema.Null,
});
const Review = Schema.Struct({
  mappingId: Schema.NullOr(Text),
  capability: Schema.NullOr(LocalKey),
  outcome: Schema.Literals(["review-required"]),
  reason: Schema.Literals([
    "capability-unmapped",
    "impact-incomplete",
    "source-mismatch",
    "catalog-changed",
    "catalog-stale",
    "policy-stale",
    "capability-not-at-head",
    "profile-forbidden",
    "mapping-conflict",
    "workload-unavailable",
    "profile-unavailable",
    "selection-unsupported",
    "reference-unavailable",
    "baseline-unavailable",
    "deployment-unbound",
  ]),
  target: Schema.NullOr(Mapping),
  ownerRoute: Schema.Null,
  deployment: Schema.Null,
});
const Context = Schema.Struct({
  change: ChangeReference,
  impact: Schema.Struct({
    apiVersion: Schema.Literals([
      "argus.dev/capability-impact/v1",
      "argus.dev/capability-impact/v2",
    ]),
    analyzerVersion: Text,
    sha256: Digest,
  }),
  catalog: Schema.Struct({
    reference: PerformanceReference,
    fingerprint: Digest,
    schemaSha256: Digest,
    scope: Schema.Struct({ issuer: Issuer, principalIssuer: Issuer, principalId: Text }),
    validUntil: Timestamp,
  }),
});
export const PerformanceSelectionContextV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/performance-selection-context/v1"]),
  ...Context.fields,
});
export const PerformanceSelectionV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/performance-selection/v1"]),
  policyVersion: Schema.Literals(["argus.dev/performance-selection-policy/v1"]),
  ...Context.fields,
  policySha256: Digest,
  reviewId: Text,
  evaluatedAt: Timestamp,
  expiresAt: Timestamp,
  ownerApprovalRequired: Schema.Literals([true]),
  status: Schema.Literals(["candidates", "review-required"]),
  decisions: Schema.Array(Schema.Union([Candidate, Omitted, Review])).check(
    Schema.isMaxLength(6001),
  ),
});
