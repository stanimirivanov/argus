import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import { ExecutionManifestV1 } from "./execution-manifest-v1.js";
import { ExecutionManifestV2 } from "./execution-manifest-v2.js";
import { ExecutionManifestV3 } from "./execution-manifest-v3.js";

const fixture: unknown = JSON.parse(
  await readFile(
    new URL("../fixtures/execution-manifest/v2/valid/browser-capability.json", import.meta.url),
    "utf8",
  ),
);
const decodeV2 = Schema.decodeUnknownEither(ExecutionManifestV2, { onExcessProperty: "error" });
const decodeV1 = Schema.decodeUnknownEither(ExecutionManifestV1, { onExcessProperty: "error" });
const decodeV3 = Schema.decodeUnknownEither(ExecutionManifestV3, { onExcessProperty: "error" });

test("browser manifest v2 accepts capability-selected UI tests", () => {
  assert.equal(decodeV2(fixture)._tag, "Right");
  assert.equal(decodeV1(fixture)._tag, "Left");
});

test("browser manifest v2 rejects functional API suite substitution", () => {
  const document = structuredClone(fixture) as { decisions: Array<{ suite: { family: string } }> };
  const decision = document.decisions[0];
  assert.ok(decision);
  decision.suite.family = "functional-api";
  assert.equal(decodeV2(document)._tag, "Left");
});

test("component-root browser manifest v3 is not interchangeable with v2", () => {
  const document = structuredClone(fixture) as {
    apiVersion: string;
    policyVersion: string;
    impact: { apiVersion: string; analyzerVersion: string };
  };
  document.apiVersion = "argus.dev/execution-manifest/v3";
  document.policyVersion = "argus.dev/selection-policy/functional-ui-component/v1";
  document.impact = {
    apiVersion: "argus.dev/component-root-impact/v1",
    analyzerVersion: "argus-component-roots/v1",
  };
  assert.equal(decodeV3(document)._tag, "Right");
  assert.equal(decodeV2(document)._tag, "Left");
  assert.equal(decodeV3(fixture)._tag, "Left");
});
