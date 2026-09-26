import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import {
  FunctionalAPIExecutionBindingsV1,
  FunctionalAPIExecutionPlanV1,
} from "./functional-api-execution-plan-v1.js";

const contractsRoot = new URL("../", import.meta.url);
test("bindings schema accepts a heterogeneous binding document", async () => {
  const input = await readFixture(
    "fixtures/functional-api-execution-bindings/v1/valid/heterogeneous.json",
  );
  const decode = Schema.decodeUnknownEither(FunctionalAPIExecutionBindingsV1, {
    onExcessProperty: "error",
  });
  assert.equal(decode(input)._tag, "Right");
});

for (const [path, valid] of [
  ["fixtures/functional-api-execution-plan/v1/valid/heterogeneous.json", true],
  ["fixtures/functional-api-execution-plan/v1/invalid/zero-test-count.json", false],
] as const) {
  test(`plan schema ${valid ? "accepts" : "rejects"} ${path}`, async () => {
    const input = await readFixture(path);
    const decode = Schema.decodeUnknownEither(FunctionalAPIExecutionPlanV1, {
      onExcessProperty: "error",
    });
    assert.equal(decode(input)._tag === "Right", valid);
  });
}

async function readFixture(path: string): Promise<unknown> {
  return JSON.parse(await readFile(new URL(path, contractsRoot), "utf8"));
}
