import { Schema } from "effect";

import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { CatalogSnapshotReference, Revision } from "./test-catalog-page-v1.js";

export const UtcTimestamp = Schema.String.check(
  Schema.isPattern(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/u),
).annotate({
  identifier: "UtcTimestamp",
  description: "An RFC 3339 instant normalized to UTC with an explicit Z suffix.",
});

export const ImpactAssertion = Schema.Literals(["supports", "refutes"]).annotate({
  identifier: "ImpactAssertion",
  description: "Whether the evidence asserts or contradicts a capability-to-test relationship.",
});

export const ImpactEvidenceType = Schema.Literals([
  "explicit",
  "static",
  "dynamic",
  "historical",
  "reviewer-confirmed",
]).annotate({
  identifier: "ImpactEvidenceType",
  description: "The method that produced one impact observation.",
});

export const ConfidenceBasisPoints = Schema.Number.check(
  Schema.isInt(),
  Schema.isBetween({ minimum: 0, maximum: 10_000 }),
).annotate({
  identifier: "ConfidenceBasisPoints",
  description:
    "Producer-specific confidence in basis points. It is evidence metadata, not a universal selection threshold.",
});

export const ImpactTestIdentity = Schema.Struct({
  testRepository: Schema.Struct({
    provider: RepositoryReference.fields.provider,
    host: RepositoryReference.fields.host,
    providerRepositoryId: RepositoryReference.fields.providerRepositoryId,
  }),
  suiteKey: LocalKey,
  testKey: LocalKey,
}).annotate({
  identifier: "ImpactTestIdentity",
  description: "A stable test identity within the catalog snapshot.",
});

export const ImpactEvidenceProducer = Schema.Struct({
  repository: RepositoryReference,
  revision: Revision,
  adapter: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
}).annotate({
  identifier: "ImpactEvidenceProducer",
  description: "The immutable producer revision and adapter responsible for the evidence bundle.",
});

export const ImpactObservation = Schema.Struct({
  key: LocalKey,
  capabilityKey: LocalKey,
  test: ImpactTestIdentity,
  assertion: ImpactAssertion,
  evidenceType: ImpactEvidenceType,
  confidenceBasisPoints: ConfidenceBasisPoints,
  rationale: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(1024)),
}).annotate({
  identifier: "ImpactObservation",
  description: "One immutable, explainable assertion about a capability-to-test relationship.",
});

export const ImpactEvidenceBundleV1APIVersion = Schema.Literals([
  "argus.dev/impact-evidence-bundle/v1",
]);

export const ImpactEvidenceBundleV1 = Schema.Struct({
  apiVersion: ImpactEvidenceBundleV1APIVersion,
  snapshot: CatalogSnapshotReference,
  producer: ImpactEvidenceProducer,
  observedAt: UtcTimestamp,
  expiresAt: Schema.NullOr(UtcTimestamp),
  observations: Schema.Array(ImpactObservation).check(
    Schema.isMinLength(1),
    Schema.isMaxLength(10_000),
  ),
}).annotate({
  identifier: "ImpactEvidenceBundleV1",
  title: "Argus impact evidence bundle v1",
  description:
    "An immutable producer observation containing capability-to-test assertions for one catalog snapshot.",
});

export type ImpactEvidenceBundleV1 = typeof ImpactEvidenceBundleV1.Type;
