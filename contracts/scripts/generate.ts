import { mkdir, readFile, writeFile } from "node:fs/promises";
import { JSONSchema } from "effect";
import { CapabilityImpactV1 } from "../source/capability-impact-v1.js";
import { ChangeSetV1 } from "../source/change-set-v1.js";
import { ImpactEdgePageV1 } from "../source/impact-edge-page-v1.js";
import { ImpactEvidenceBundleV1 } from "../source/impact-evidence-bundle-v1.js";
import { RepositoryDescriptorV1 } from "../source/repository-descriptor-v1.js";
import { TestCatalogPageV1 } from "../source/test-catalog-page-v1.js";

const artifacts = [
  {
    name: "capability impact",
    outputPath: new URL(
      "../generated/capability-impact/v1/capability-impact.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/capability-impact/v1/schema.json",
      ...JSONSchema.make(CapabilityImpactV1, { target: "jsonSchema2020-12" }),
    },
  },
  {
    name: "change set",
    outputPath: new URL("../generated/change-set/v1/change-set.schema.json", import.meta.url),
    document: {
      $id: "https://argus.dev/contracts/change-set/v1/schema.json",
      ...JSONSchema.make(ChangeSetV1, { target: "jsonSchema2020-12" }),
    },
  },
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
  {
    name: "impact evidence bundle",
    outputPath: new URL(
      "../generated/impact-evidence-bundle/v1/impact-evidence-bundle.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/impact-evidence-bundle/v1/schema.json",
      ...JSONSchema.make(ImpactEvidenceBundleV1, { target: "jsonSchema2020-12" }),
    },
  },
  {
    name: "impact edge page",
    outputPath: new URL(
      "../generated/impact-edge-page/v1/impact-edge-page.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/impact-edge-page/v1/schema.json",
      ...JSONSchema.make(ImpactEdgePageV1, { target: "jsonSchema2020-12" }),
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
