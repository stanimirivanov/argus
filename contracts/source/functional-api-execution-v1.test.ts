import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import {
  ExecutionAttemptV1,
  FunctionalAPIAdapterRequestV1,
  FunctionalAPIAdapterResultV1,
} from "./functional-api-execution-v1.js";

const contractsRoot = new URL("../", import.meta.url);
const cases = [
  {
    name: "adapter request",
    path: "fixtures/functional-api-adapter-request/v1/valid/selected.json",
    schema: FunctionalAPIAdapterRequestV1,
    valid: true,
  },
  {
    name: "adapter result",
    path: "fixtures/functional-api-adapter-result/v1/valid/passed.json",
    schema: FunctionalAPIAdapterResultV1,
    valid: true,
  },
  {
    name: "adapter result with unknown outcome",
    path: "fixtures/functional-api-adapter-result/v1/invalid/unknown-outcome.json",
    schema: FunctionalAPIAdapterResultV1,
    valid: false,
  },
  {
    name: "execution attempt",
    path: "fixtures/execution-attempt/v1/valid/passed.json",
    schema: ExecutionAttemptV1,
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
