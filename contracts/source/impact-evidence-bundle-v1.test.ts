import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import { ImpactEvidenceBundleV1 } from "./impact-evidence-bundle-v1.js";

interface Fixture {
  readonly path: string;
  readonly schemaValid: boolean;
}

interface FixtureManifest {
  readonly fixtures: ReadonlyArray<Fixture>;
}

const contractsRoot = new URL("../", import.meta.url);
const manifest = JSON.parse(
  await readFile(
    new URL("fixtures/impact-evidence-bundle/v1/manifest.json", contractsRoot),
    "utf8",
  ),
) as FixtureManifest;
const decode = Schema.decodeUnknownEither(ImpactEvidenceBundleV1, { onExcessProperty: "error" });

for (const fixture of manifest.fixtures) {
  test(`Effect Schema ${fixture.schemaValid ? "accepts" : "rejects"} ${fixture.path}`, async () => {
    const input: unknown = JSON.parse(await readFile(new URL(fixture.path, contractsRoot), "utf8"));
    assert.equal(decode(input)._tag === "Right", fixture.schemaValid);
  });
}
