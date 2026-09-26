import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import { SelectionShadowReportV1 } from "./selection-shadow-report-v1.js";

const contractsRoot = new URL("../", import.meta.url);
const cases = [
  ["accepts", "fixtures/selection-shadow-report/v1/valid/missed-failure.json", true],
  ["rejects", "fixtures/selection-shadow-report/v1/invalid/invalid-recall.json", false],
] as const;
const decode = Schema.decodeUnknownEither(SelectionShadowReportV1, { onExcessProperty: "error" });

for (const [verb, path, valid] of cases) {
  test(`Effect Schema ${verb} ${path}`, async () => {
    const input: unknown = JSON.parse(await readFile(new URL(path, contractsRoot), "utf8"));
    assert.equal(decode(input)._tag === "Right", valid);
  });
}
