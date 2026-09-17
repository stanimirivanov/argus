import { mkdir, readFile, writeFile } from "node:fs/promises";
import { JSONSchema } from "effect";

import { RepositoryDescriptorV1 } from "../source/repository-descriptor-v1.js";
import { TestCatalogPageV1 } from "../source/test-catalog-page-v1.js";

const artifacts = [
  {
    name: "repository descriptor",
    outputPath: new URL(
      "../generated/repository-descriptor/v1/repository-descriptor.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/repository-descriptor/v1/schema.json",
      ...JSONSchema.make(RepositoryDescriptorV1, { target: "jsonSchema2020-12" }),
    },
  },
  {
    name: "test catalog page",
    outputPath: new URL(
      "../generated/test-catalog-page/v1/test-catalog-page.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/test-catalog-page/v1/schema.json",
      ...JSONSchema.make(TestCatalogPageV1, { target: "jsonSchema2020-12" }),
    },
  },
] as const;

const check = process.argv.includes("--check");
for (const artifact of artifacts) {
  const expected = `${JSON.stringify(artifact.document, undefined, 2)}\n`;
  if (check) {
    const actual = await readFile(artifact.outputPath, "utf8");
    if (actual !== expected) {
      throw new Error(`generated ${artifact.name} schema is stale; run make generate-contracts`);
    }
  } else {
    await mkdir(new URL("./", artifact.outputPath), { recursive: true });
    await writeFile(artifact.outputPath, expected, "utf8");
  }
}

console.log(
  check
    ? "Verified generated contract JSON Schemas are reproducible."
    : "Generated contract JSON Schemas from Effect Schema.",
);
