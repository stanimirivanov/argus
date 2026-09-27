import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import {
  AdaptationProposalV1,
  FunctionalAPIAdaptationRequestV1,
  FunctionalAPIAdaptationResultV1,
} from "./adaptation-v1.js";

const contractsRoot = new URL("../", import.meta.url);
const cases = [
  {
    name: "adaptation request",
    path: "fixtures/functional-api-adaptation-request/v1/valid/endpoint-rename.json",
    schema: FunctionalAPIAdaptationRequestV1,
    valid: true,
  },
  {
    name: "adaptation result",
    path: "fixtures/functional-api-adaptation-result/v1/valid/candidate.json",
    schema: FunctionalAPIAdaptationResultV1,
    valid: true,
  },
  {
    name: "adaptation result with assertion role",
    path: "fixtures/functional-api-adaptation-result/v1/invalid/assertion-edit.json",
    schema: FunctionalAPIAdaptationResultV1,
    valid: false,
  },
  {
    name: "adaptation proposal",
    path: "fixtures/adaptation-proposal/v1/valid/endpoint-rename.json",
    schema: AdaptationProposalV1,
    valid: true,
  },
] as const;

for (const fixture of cases) {
  test(`Effect Schema ${fixture.valid ? "accepts" : "rejects"} ${fixture.name}`, async () => {
    const input: unknown = JSON.parse(await readFile(new URL(fixture.path, contractsRoot), "utf8"));
    const decode = Schema.decodeUnknownEither(fixture.schema as Schema.Schema.AnyNoContext, {
      onExcessProperty: "error",
    });
    assert.equal(decode(input)._tag === "Right", fixture.valid);
  });
}
