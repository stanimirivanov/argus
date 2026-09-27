import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import { AdaptationReviewV1 } from "./adaptation-review-v1.js";

const contractsRoot = new URL("../", import.meta.url);

test("Effect Schema accepts a draft adaptation review", async () => {
  const input: unknown = JSON.parse(
    await readFile(
      new URL("fixtures/adaptation-review/v1/valid/draft-endpoint-repair.json", contractsRoot),
      "utf8",
    ),
  );
  const decode = Schema.decodeUnknownEither(AdaptationReviewV1, { onExcessProperty: "error" });
  assert.equal(decode(input)._tag, "Right");
});

test("Effect Schema rejects a non-draft adaptation review", async () => {
  const input: unknown = JSON.parse(
    await readFile(
      new URL("fixtures/adaptation-review/v1/invalid/non-draft.json", contractsRoot),
      "utf8",
    ),
  );
  const decode = Schema.decodeUnknownEither(AdaptationReviewV1, { onExcessProperty: "error" });
  assert.equal(decode(input)._tag, "Left");
});
