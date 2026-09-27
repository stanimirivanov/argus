import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import { ReviewOutcomeV1 } from "./review-outcome-v1.js";

const contractsRoot = new URL("../", import.meta.url);

for (const fixture of [
  {
    name: "edited accepted outcome",
    path: "fixtures/review-outcome/v1/valid/accepted-with-edits.json",
    valid: true,
  },
  {
    name: "unknown review decision",
    path: "fixtures/review-outcome/v1/invalid/unknown-decision.json",
    valid: false,
  },
] as const) {
  test(`Effect Schema ${fixture.valid ? "accepts" : "rejects"} ${fixture.name}`, async () => {
    const input: unknown = JSON.parse(await readFile(new URL(fixture.path, contractsRoot), "utf8"));
    const decode = Schema.decodeUnknownEither(ReviewOutcomeV1, { onExcessProperty: "error" });
    assert.equal(decode(input)._tag === "Right", fixture.valid);
  });
}
