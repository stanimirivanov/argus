import { Schema } from "effect";

import {
  DisplayName,
  LocalKey,
  RepositoryDescriptorV1APIVersion,
  RepositoryReference,
  TestFamily,
} from "./repository-descriptor-v1.js";

const HexSHA1 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{40}$/u));
const HexSHA256 = Schema.String.check(Schema.isPattern(/^[0-9a-f]{64}$/u));

export const Revision = Schema.Union([
  Schema.Struct({ algorithm: Schema.Literals(["git-sha1"]), digest: HexSHA1 }),
  Schema.Struct({ algorithm: Schema.Literals(["git-sha256"]), digest: HexSHA256 }),
]).annotate({
  identifier: "Revision",
  description: "An immutable, normalized Git object revision.",
});

export const CatalogSnapshotReference = Schema.Struct({
  sourceRepository: RepositoryReference,
  revision: Revision,
  descriptorApiVersion: RepositoryDescriptorV1APIVersion,
}).annotate({
  identifier: "CatalogSnapshotReference",
  description: "The immutable repository descriptor observation queried by this result page.",
});

export const CapabilityReference = Schema.Struct({
  key: LocalKey,
  name: DisplayName,
}).annotate({ identifier: "CapabilityReference" });

export const TestCatalogEntry = Schema.Struct({
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
  capabilities: Schema.Array(CapabilityReference).check(Schema.isMinLength(1)),
}).annotate({
  identifier: "TestCatalogEntry",
  description:
    "A stable test identity and its immutable catalog metadata. Identity is testRepository identity plus suite.key plus test.key.",
});

export const ContinuationCursor = Schema.String.check(
  Schema.isMinLength(1),
  Schema.isMaxLength(4096),
  Schema.isPattern(/^[A-Za-z0-9_-]+$/u),
).annotate({
  identifier: "ContinuationCursor",
  description: "An opaque, versioned continuation token bound to the originating query.",
});

export const TestCatalogPageV1APIVersion = Schema.Literals(["argus.dev/test-catalog-page/v1"]);

export const TestCatalogPageV1 = Schema.Struct({
  apiVersion: TestCatalogPageV1APIVersion,
  snapshot: CatalogSnapshotReference,
  items: Schema.Array(TestCatalogEntry),
  nextCursor: Schema.NullOr(ContinuationCursor),
}).annotate({
  identifier: "TestCatalogPageV1",
  title: "Argus test catalog page v1",
  description:
    "A deterministic page of tests from one immutable catalog snapshot, ordered by stable test identity.",
});

export type TestCatalogPageV1 = typeof TestCatalogPageV1.Type;
