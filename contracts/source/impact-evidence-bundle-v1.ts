import { Schema } from "effect";

import { LocalKey, RepositoryReference } from "./repository-descriptor-v1.js";
import { CatalogSnapshotReference, Revision } from "./test-catalog-page-v1.js";

export const UtcTimestamp = Schema.String.pipe(
  Schema.pattern(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/),
).annotations({
  identifier: "UtcTimestamp",
  description: "An RFC 3339 instant normalized to UTC with an explicit Z suffix.",
});

export const ImpactAssertion = Schema.Literal("supports", "refutes").annotations({
  identifier: "ImpactAssertion",
  description: "Whether the evidence asserts or contradicts a capability-to-test relationship.",
});

export const ImpactEvidenceType = Schema.Literal(
  "explicit",
  "static",
  "dynamic",
  "historical",
  "reviewer-confirmed",
).annotations({
  identifier: "ImpactEvidenceType",
  description: "The method that produced one impact observation.",
});

export const ConfidenceBasisPoints = Schema.Number.pipe(
  Schema.int(),
  Schema.between(0, 10_000),
).annotations({
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
}).annotations({
  identifier: "ImpactTestIdentity",
  description: "A stable test identity within the catalog snapshot.",
});

export const ImpactEvidenceProducer = Schema.Struct({
  repository: RepositoryReference,
  revision: Revision,
  adapter: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(127)),
}).annotations({
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
  rationale: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(1024)),
}).annotations({
  identifier: "ImpactObservation",
  description: "One immutable, explainable assertion about a capability-to-test relationship.",
});

export const ImpactEvidenceBundleV1APIVersion = Schema.Literal(
  "argus.dev/impact-evidence-bundle/v1",
);

export const ImpactEvidenceBundleV1 = Schema.Struct({
  apiVersion: ImpactEvidenceBundleV1APIVersion,
  snapshot: CatalogSnapshotReference,
  producer: ImpactEvidenceProducer,
  observedAt: UtcTimestamp,
  expiresAt: Schema.NullOr(UtcTimestamp),
  observations: Schema.Array(ImpactObservation).pipe(Schema.minItems(1), Schema.maxItems(10_000)),
}).annotations({
  identifier: "ImpactEvidenceBundleV1",
  title: "Argus impact evidence bundle v1",
  description:
    "An immutable producer observation containing capability-to-test assertions for one catalog snapshot.",
});

export type ImpactEvidenceBundleV1 = typeof ImpactEvidenceBundleV1.Type;
