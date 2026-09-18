import { Schema } from "effect";
import {
  ConfidenceBasisPoints,
  ImpactAssertion,
  ImpactEvidenceProducer,
  ImpactEvidenceType,
  UtcTimestamp,
} from "./impact-evidence-bundle-v1.js";
import {
  DisplayName,
  LocalKey,
  RepositoryReference,
  TestFamily,
} from "./repository-descriptor-v1.js";
import {
  CapabilityReference,
  CatalogSnapshotReference,
  ContinuationCursor,
} from "./test-catalog-page-v1.js";

export const ImpactEvidenceState = Schema.Literal("active", "expired").annotations({
  identifier: "ImpactEvidenceState",
});

export const ImpactEvidenceReference = Schema.Struct({
  producer: ImpactEvidenceProducer,
  observationKey: LocalKey,
  assertion: ImpactAssertion,
  evidenceType: ImpactEvidenceType,
  confidenceBasisPoints: ConfidenceBasisPoints,
  rationale: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(1024)),
  observedAt: UtcTimestamp,
  expiresAt: Schema.NullOr(UtcTimestamp),
  state: ImpactEvidenceState,
}).annotations({
  identifier: "ImpactEvidenceReference",
  description: "One visible evidence observation and its state at the requested evaluation time.",
});

export const ImpactEdgeStatus = Schema.Literal(
  "supported",
  "refuted",
  "stale",
  "conflicting",
).annotations({ identifier: "ImpactEdgeStatus" });

export const MappingConflict = Schema.Struct({
  kind: Schema.Literal("contradictory-active-assertions"),
  supportingEvidenceCount: Schema.Number.pipe(Schema.int(), Schema.greaterThanOrEqualTo(1)),
  refutingEvidenceCount: Schema.Number.pipe(Schema.int(), Schema.greaterThanOrEqualTo(1)),
}).annotations({
  identifier: "MappingConflict",
  description:
    "A reviewable conflict produced when active evidence both supports and refutes the same edge.",
});

export const ImpactEdge = Schema.Struct({
  capability: CapabilityReference,
  testRepository: RepositoryReference,
  suite: Schema.Struct({
    key: LocalKey,
    family: TestFamily,
    adapter: Schema.String.pipe(Schema.minLength(1), Schema.maxLength(127)),
  }),
  test: Schema.Struct({
    key: LocalKey,
    name: DisplayName,
  }),
  status: ImpactEdgeStatus,
  evidence: Schema.Array(ImpactEvidenceReference).pipe(Schema.minItems(1)),
  conflict: Schema.NullOr(MappingConflict),
}).annotations({
  identifier: "ImpactEdge",
  description: "A capability-to-test relationship with all visible evidence and its derived state.",
});

export const ImpactEdgePageV1APIVersion = Schema.Literal("argus.dev/impact-edge-page/v1");

export const ImpactEdgePageV1 = Schema.Struct({
  apiVersion: ImpactEdgePageV1APIVersion,
  snapshot: CatalogSnapshotReference,
  evaluatedAt: UtcTimestamp,
  items: Schema.Array(ImpactEdge),
  nextCursor: Schema.NullOr(ContinuationCursor),
}).annotations({
  identifier: "ImpactEdgePageV1",
  title: "Argus impact edge page v1",
  description:
    "A deterministic page of capability-to-test relationships evaluated at an explicit UTC instant.",
});

export type ImpactEdgePageV1 = typeof ImpactEdgePageV1.Type;
