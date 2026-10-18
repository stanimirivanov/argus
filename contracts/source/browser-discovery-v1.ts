import { Schema } from "effect";

import { LocalKey, RepositoryIdentity, RepositoryReference } from "./repository-descriptor-v1.js";
import { CatalogSnapshotReference, Revision } from "./test-catalog-page-v1.js";
import { Timestamp } from "./timestamp.js";

const BoundedText = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(127));
const SourceRoot = Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(1024));

const BrowserCase = Schema.Struct({
  testKey: LocalKey,
  project: BoundedText,
  file: SourceRoot,
  title: Schema.String.check(Schema.isMinLength(1), Schema.isMaxLength(255)),
  tags: Schema.Array(BoundedText).check(Schema.isMaxLength(64)),
  owners: Schema.Array(BoundedText).check(Schema.isMaxLength(64)),
});

const BrowserSuiteObservation = Schema.Struct({
  testRepository: RepositoryReference,
  revision: Revision,
  suiteKey: LocalKey,
  cases: Schema.Array(BrowserCase).check(Schema.isMaxLength(10000)),
});

const BrowserMapping = Schema.Struct({
  kind: Schema.Literals(["route", "component"]),
  key: LocalKey,
  routePattern: Schema.String.check(Schema.isMaxLength(512)),
  sourceRoot: SourceRoot,
  testRepository: RepositoryIdentity,
  suiteKey: LocalKey,
  testKey: LocalKey,
});

/**
 * A complete observation requires all declared Playwright UI suites and tests.
 * Mappings are reviewed source relationships; observed tags and titles are not
 * selection authority. Semantic completeness and source-root rules are checked
 * against the immutable descriptor by the Go ingress.
 */
export const BrowserDiscoveryV1 = Schema.Struct({
  apiVersion: Schema.Literals(["argus.dev/browser-discovery/v1"]),
  observationId: LocalKey,
  catalog: CatalogSnapshotReference,
  collectorVersion: BoundedText,
  collectedAt: Timestamp,
  expiresAt: Timestamp,
  complete: Schema.Boolean,
  mappingRevision: Revision,
  mappingComplete: Schema.Boolean,
  suites: Schema.Array(BrowserSuiteObservation).check(Schema.isMaxLength(10000)),
  mappings: Schema.Array(BrowserMapping).check(Schema.isMaxLength(20000)),
}).annotate({
  identifier: "BrowserDiscoveryV1",
  description:
    "Immutable Playwright inventory and explicit route/component-to-test mapping evidence.",
});

export type BrowserDiscoveryV1 = typeof BrowserDiscoveryV1.Type;
