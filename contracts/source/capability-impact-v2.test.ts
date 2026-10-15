import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";
import { CapabilityImpactV2 } from "./capability-impact-v2.js";

const root = new URL("../", import.meta.url);
const manifest = JSON.parse(
  await readFile(new URL("fixtures/capability-impact/v2/manifest.json", root), "utf8"),
) as { readonly fixtures: ReadonlyArray<{ readonly path: string; readonly schemaValid: boolean }> };
const decode = Schema.decodeUnknownResult(CapabilityImpactV2, { onExcessProperty: "error" });

for (const fixture of manifest.fixtures) {
  test(`capability impact v2 ${fixture.schemaValid ? "accepts" : "rejects"} ${fixture.path}`, async () => {
    const input: unknown = JSON.parse(await readFile(new URL(fixture.path, root), "utf8"));
    assert.equal(decode(input)._tag === "Success", fixture.schemaValid);
  });
}
