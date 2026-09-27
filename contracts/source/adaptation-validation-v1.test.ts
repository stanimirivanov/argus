import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import {
  FunctionalAPIRepairValidationRequestV1,
  FunctionalAPIRepairValidationResultV1,
  ValidationEvidenceV1,
} from "./adaptation-validation-v1.js";

const contractsRoot = new URL("../", import.meta.url);
const cases = [
  {
    name: "repair validation request",
    path: "fixtures/functional-api-repair-validation-request/v1/valid/original.json",
    schema: FunctionalAPIRepairValidationRequestV1,
    valid: true,
  },
  {
    name: "repair validation result",
    path: "fixtures/functional-api-repair-validation-result/v1/valid/original-failed.json",
    schema: FunctionalAPIRepairValidationResultV1,
    valid: true,
  },
  {
    name: "repair validation result with unknown phase",
    path: "fixtures/functional-api-repair-validation-result/v1/invalid/unknown-phase.json",
    schema: FunctionalAPIRepairValidationResultV1,
    valid: false,
  },
  {
    name: "validation evidence",
    path: "fixtures/validation-evidence/v1/valid/endpoint-rename.json",
    schema: ValidationEvidenceV1,
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
