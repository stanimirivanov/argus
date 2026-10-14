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

export const ImpactEvidenceState = Schema.Literals(["active", "expired"]).annotate({
  identifier: "ImpactEvidenceState",
});

export const ImpactEvidenceReference = Schema.Struct({
  producer: ImpactEvidenceProducer,
  observationKey: LocalKey,
  assertion: ImpactAssertion,
  evidenceType: ImpactEvidenceType,
  confidenceBasisPoints: ConfidenceBasisPoints,
  rationale: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(1024)),
  observedAt: UtcTimestamp,
  expiresAt: Schema.NullOr(UtcTimestamp),
  state: ImpactEvidenceState,
}).annotate({
  identifier: "ImpactEvidenceReference",
  description: "One visible evidence observation and its state at the requested evaluation time.",
});

export const ImpactEdgeStatus = Schema.Literals([
  "supported",
  "refuted",
  "stale",
  "conflicting",
]).annotate({ identifier: "ImpactEdgeStatus" });

export const MappingConflict = Schema.Struct({
  kind: Schema.Literals(["contradictory-active-assertions"]),
  supportingEvidenceCount: Schema.Number.check(Schema.isInt(), Schema.isGreaterThanOrEqualTo(1)),
  refutingEvidenceCount: Schema.Number.check(Schema.isInt(), Schema.isGreaterThanOrEqualTo(1)),
}).annotate({
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
    adapter: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127)),
  }),
  test: Schema.Struct({
    key: LocalKey,
    name: DisplayName,
  }),
  status: ImpactEdgeStatus,
  evidence: Schema.Array(ImpactEvidenceReference).check(Schema.isMinLength(1)),
  conflict: Schema.NullOr(MappingConflict),
}).annotate({
  identifier: "ImpactEdge",
  description: "A capability-to-test relationship with all visible evidence and its derived state.",
});

export const ImpactEdgePageV1APIVersion = Schema.Literals(["argus.dev/impact-edge-page/v1"]);

export const ImpactEdgePageV1 = Schema.Struct({
  apiVersion: ImpactEdgePageV1APIVersion,
  snapshot: CatalogSnapshotReference,
  evaluatedAt: UtcTimestamp,
  items: Schema.Array(ImpactEdge),
  nextCursor: Schema.NullOr(ContinuationCursor),
}).annotate({
  identifier: "ImpactEdgePageV1",
  title: "Argus impact edge page v1",
  description:
    "A deterministic page of capability-to-test relationships evaluated at an explicit UTC instant.",
});

export type ImpactEdgePageV1 = typeof ImpactEdgePageV1.Type;
