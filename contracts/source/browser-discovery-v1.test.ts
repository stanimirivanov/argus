import assert from "node:assert/strict";
import test from "node:test";
import { Schema } from "effect";

import { BrowserDiscoveryV1 } from "./browser-discovery-v1.js";
import { BrowserSelectionManifestV1 } from "./browser-selection-manifest-v1.js";
import { BrowserSuiteBindingsV1 } from "./browser-suite-bindings-v1.js";

const source = { provider: "github", host: "github.com", providerRepositoryId: "source-1" };
const tests = { provider: "github", host: "github.com", providerRepositoryId: "tests-1" };
const sourceRef = { ...source, owner: "sample", name: "web" };
const testsRef = { ...tests, owner: "sample", name: "web-tests" };
const base = { algorithm: "git-sha1", digest: "1".repeat(40) };
const head = { algorithm: "git-sha1", digest: "2".repeat(40) };
const testRevision = { algorithm: "git-sha1", digest: "3".repeat(40) };
const catalog = {
  sourceRepository: sourceRef,
  revision: base,
  descriptorApiVersion: "argus.dev/repository-descriptor/v1",
};

test("browser discovery requires explicit project variants and mapping fields", () => {
  const decode = Schema.decodeUnknownResult(BrowserDiscoveryV1, { onExcessProperty: "error" });
  const document = {
    apiVersion: "argus.dev/browser-discovery/v1",
    observationId: "ci-123",
    catalog,
    collectorVersion: "playwright-1.60",
    collectedAt: "2026-10-18T06:00:00Z",
    expiresAt: "2026-10-19T06:00:00Z",
    complete: true,
    mappingRevision: base,
    mappingComplete: true,
    suites: [
      {
        testRepository: testsRef,
        revision: testRevision,
        suiteKey: "web-ui",
        cases: [
          {
            testKey: "happy",
            project: "chromium",
            file: "tests/web.spec.ts",
            title: "happy",
            tags: [],
            owners: [],
          },
        ],
      },
    ],
    mappings: [
      {
        kind: "route",
        key: "checkout",
        routePattern: "/checkout",
        sourceRoot: "web/routes/checkout",
        testRepository: tests,
        suiteKey: "web-ui",
        testKey: "happy",
      },
    ],
  };
  const firstSuite = document.suites[0];
  assert.ok(firstSuite);
  assert.equal(decode(document)._tag, "Success");
  assert.equal(
    decode({
      ...document,
      suites: [
        {
          ...firstSuite,
          revision: base,
          cases: [
            {
              ...firstSuite.cases[0],
              project: "",
            },
          ],
        },
      ],
    })._tag,
    "Failure",
  );
  assert.equal(
    decode({ ...document, mappings: [{ ...document.mappings[0], routePattern: undefined }] })._tag,
    "Failure",
  );
});

test("browser checkout bindings and manifest use distinct versions", () => {
  const decodeBindings = Schema.decodeUnknownResult(BrowserSuiteBindingsV1, {
    onExcessProperty: "error",
  });
  const bindings = {
    apiVersion: "argus.dev/browser-suite-bindings/v1",
    sourceRepository: source,
    suites: [{ testRepository: tests, suiteKey: "web-ui", revision: testRevision }],
  };
  assert.equal(decodeBindings(bindings)._tag, "Success");
  assert.equal(decodeBindings({ ...bindings, suites: [] })._tag, "Failure");

  const decodeManifest = Schema.decodeUnknownResult(BrowserSelectionManifestV1, {
    onExcessProperty: "error",
  });
  const manifest = {
    apiVersion: "argus.dev/browser-selection-manifest/v1",
    policyVersion: "argus.dev/selection-policy/browser-mapped/v1",
    change: {
      sourceRepository: sourceRef,
      pullRequest: { number: 42 },
      baseRevision: base,
      headRevision: head,
      observedAt: "2026-10-18T06:00:00Z",
      trigger: {
        provider: "github",
        deliveryId: "delivery-42",
        event: "pull_request",
        action: "synchronize",
      },
    },
    catalog,
    changedPaths: ["web/routes/checkout/page.tsx"],
    pathsComplete: true,
    evidence: null,
    evaluatedAt: "2026-10-18T07:00:00Z",
    family: "functional-ui",
    mode: "fallback",
    decisions: [
      {
        testRepository: testsRef,
        suiteKey: "web-ui",
        testKey: "happy",
        name: "Happy",
        revision: testRevision,
        projects: ["chromium"],
        outcome: "RUN_REQUIRED",
        remainingExecution: "none",
        reason: "CONSERVATIVE_FALLBACK",
        mappingKeys: [],
      },
    ],
    warnings: ["browser discovery is missing"],
  };
  assert.equal(decodeManifest(manifest)._tag, "Success");
  assert.equal(
    decodeManifest({ ...manifest, apiVersion: "argus.dev/execution-manifest/v1" })._tag,
    "Failure",
  );
});
