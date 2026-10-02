import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import { ExecutionManifestV1 } from "./execution-manifest-v1.js";
import { ExecutionManifestV2 } from "./execution-manifest-v2.js";

const fixture: unknown = JSON.parse(
  await readFile(
    new URL("../fixtures/execution-manifest/v2/valid/browser-capability.json", import.meta.url),
    "utf8",
  ),
);
const decodeV2 = Schema.decodeUnknownEither(ExecutionManifestV2, { onExcessProperty: "error" });
const decodeV1 = Schema.decodeUnknownEither(ExecutionManifestV1, { onExcessProperty: "error" });

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
