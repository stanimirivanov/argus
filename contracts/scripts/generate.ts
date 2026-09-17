import { mkdir, readFile, writeFile } from "node:fs/promises";
import { JSONSchema } from "effect";

import { RepositoryDescriptorV1 } from "../source/repository-descriptor-v1.js";

const outputPath = new URL(
  "../generated/repository-descriptor/v1/repository-descriptor.schema.json",
  import.meta.url,
);

const generated = JSONSchema.make(RepositoryDescriptorV1, {
  target: "jsonSchema2020-12",
});
const document = {
  $id: "https://argus.dev/contracts/repository-descriptor/v1/schema.json",
  ...generated,
};
const expected = `${JSON.stringify(document, undefined, 2)}\n`;

if (process.argv.includes("--check")) {
  const actual = await readFile(outputPath, "utf8");
  if (actual !== expected) {
    throw new Error("generated repository descriptor schema is stale; run make generate-contracts");
  }
  console.log("Verified the repository descriptor JSON Schema is reproducible.");
} else {
  await mkdir(new URL("./", outputPath), { recursive: true });
  await writeFile(outputPath, expected, "utf8");
  console.log("Generated the repository descriptor JSON Schema from Effect Schema.");
}
