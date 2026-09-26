import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import {
  ExecutionPlanAttemptBindingsV1,
  SelectionPlanShadowReportV1,
} from "./selection-plan-shadow-report-v1.js";

const contractsRoot = new URL("../", import.meta.url);

for (const [path, valid] of [
  ["fixtures/execution-plan-attempt-bindings/v1/valid/heterogeneous.json", true],
  ["fixtures/execution-plan-attempt-bindings/v1/invalid/invalid-attempt-id.json", false],
] as const) {
  test(`attempt bindings ${valid ? "accept" : "reject"} ${path}`, async () => {
    const input = await readFixture(path);
    const decode = Schema.decodeUnknownEither(ExecutionPlanAttemptBindingsV1, {
      onExcessProperty: "error",
    });
    assert.equal(decode(input)._tag === "Right", valid);
  });
}

for (const [path, valid] of [
  ["fixtures/selection-plan-shadow-report/v1/valid/full-only-miss.json", true],
  ["fixtures/selection-plan-shadow-report/v1/invalid/invalid-recall.json", false],
] as const) {
  test(`plan shadow schema ${valid ? "accepts" : "rejects"} ${path}`, async () => {
    const input = await readFixture(path);
    const decode = Schema.decodeUnknownEither(SelectionPlanShadowReportV1, {
      onExcessProperty: "error",
    });
    assert.equal(decode(input)._tag === "Right", valid);
  });
}

async function readFixture(path: string): Promise<unknown> {
  return JSON.parse(await readFile(new URL(path, contractsRoot), "utf8"));
}
