import { mkdir, readFile, writeFile } from "node:fs/promises";
import { AdaptationReviewV1 } from "../source/adaptation-review-v1.js";
import {
  AdaptationProposalV1,
  FunctionalAPIAdaptationRequestV1,
  FunctionalAPIAdaptationResultV1,
} from "../source/adaptation-v1.js";
import {
  FunctionalAPIRepairValidationRequestV1,
  FunctionalAPIRepairValidationResultV1,
  ValidationEvidenceV1,
  ValidationRejectionV1,
} from "../source/adaptation-validation-v1.js";
import { CapabilityImpactV1 } from "../source/capability-impact-v1.js";
import { CapabilityImpactV2 } from "../source/capability-impact-v2.js";
import { ChangeSetV1 } from "../source/change-set-v1.js";
import { ExecutionManifestV1 } from "../source/execution-manifest-v1.js";
import { ExecutionManifestV2 } from "../source/execution-manifest-v2.js";
import { ExecutionManifestV3 } from "../source/execution-manifest-v3.js";
import { ExecutionManifestV4 } from "../source/execution-manifest-v4.js";
import {
  FunctionalAPIExecutionBindingsV1,
  FunctionalAPIExecutionPlanV1,
} from "../source/functional-api-execution-plan-v1.js";
import {
  ExecutionAttemptV1,
  FunctionalAPIAdapterRequestV1,
  FunctionalAPIAdapterResultV1,
} from "../source/functional-api-execution-v1.js";
import { HeadTestInventoryV1 } from "../source/head-test-inventory-v1.js";
import { ImpactEdgePageV1 } from "../source/impact-edge-page-v1.js";
import { ImpactEvidenceBundleV1 } from "../source/impact-evidence-bundle-v1.js";
import { RepositoryDescriptorV1 } from "../source/repository-descriptor-v1.js";
import { ReviewOutcomeV1 } from "../source/review-outcome-v1.js";
import { ReviewedSelectionPolicyV1 } from "../source/reviewed-selection-policy-v1.js";
import {
  ExecutionPlanAttemptBindingsV1,
  SelectionPlanShadowReportV1,
} from "../source/selection-plan-shadow-report-v1.js";
import { SelectionShadowReportV1 } from "../source/selection-shadow-report-v1.js";
import { TestCatalogPageV1 } from "../source/test-catalog-page-v1.js";
import { compileJsonSchema } from "./json-schema.js";

const artifacts = [
  {
    name: "validation rejection",
    outputPath: new URL(
      "../generated/validation-rejection/v1/validation-rejection.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/validation-rejection/v1/schema.json",
      ...compileJsonSchema(ValidationRejectionV1),
    },
  },
  {
    name: "review outcome",
    outputPath: new URL(
      "../generated/review-outcome/v1/review-outcome.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/review-outcome/v1/schema.json",
      ...compileJsonSchema(ReviewOutcomeV1),
    },
  },
  {
    name: "adaptation review",
    outputPath: new URL(
      "../generated/adaptation-review/v1/adaptation-review.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/adaptation-review/v1/schema.json",
      ...compileJsonSchema(AdaptationReviewV1),
    },
  },
  {
    name: "functional API repair validation request",
    outputPath: new URL(
      "../generated/functional-api-repair-validation-request/v1/functional-api-repair-validation-request.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/functional-api-repair-validation-request/v1/schema.json",
      ...compileJsonSchema(FunctionalAPIRepairValidationRequestV1),
    },
  },
  {
    name: "functional API repair validation result",
    outputPath: new URL(
      "../generated/functional-api-repair-validation-result/v1/functional-api-repair-validation-result.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/functional-api-repair-validation-result/v1/schema.json",
      ...compileJsonSchema(FunctionalAPIRepairValidationResultV1),
    },
  },
  {
    name: "validation evidence",
    outputPath: new URL(
      "../generated/validation-evidence/v1/validation-evidence.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/validation-evidence/v1/schema.json",
      ...compileJsonSchema(ValidationEvidenceV1),
    },
  },
  {
    name: "functional API adaptation request",
    outputPath: new URL(
      "../generated/functional-api-adaptation-request/v1/functional-api-adaptation-request.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/functional-api-adaptation-request/v1/schema.json",
      ...compileJsonSchema(FunctionalAPIAdaptationRequestV1),
    },
  },
  {
    name: "functional API adaptation result",
    outputPath: new URL(
      "../generated/functional-api-adaptation-result/v1/functional-api-adaptation-result.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/functional-api-adaptation-result/v1/schema.json",
      ...compileJsonSchema(FunctionalAPIAdaptationResultV1),
    },
  },
  {
    name: "adaptation proposal",
    outputPath: new URL(
      "../generated/adaptation-proposal/v1/adaptation-proposal.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/adaptation-proposal/v1/schema.json",
      ...compileJsonSchema(AdaptationProposalV1),
    },
  },
  {
    name: "execution plan attempt bindings",
    outputPath: new URL(
      "../generated/execution-plan-attempt-bindings/v1/execution-plan-attempt-bindings.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/execution-plan-attempt-bindings/v1/schema.json",
      ...compileJsonSchema(ExecutionPlanAttemptBindingsV1),
    },
  },
  {
    name: "selection plan shadow report",
    outputPath: new URL(
      "../generated/selection-plan-shadow-report/v1/selection-plan-shadow-report.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/selection-plan-shadow-report/v1/schema.json",
      ...compileJsonSchema(SelectionPlanShadowReportV1),
    },
  },
  {
    name: "functional API execution bindings",
    outputPath: new URL(
      "../generated/functional-api-execution-bindings/v1/functional-api-execution-bindings.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/functional-api-execution-bindings/v1/schema.json",
      ...compileJsonSchema(FunctionalAPIExecutionBindingsV1),
    },
  },
  {
    name: "functional API execution plan",
    outputPath: new URL(
      "../generated/functional-api-execution-plan/v1/functional-api-execution-plan.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/functional-api-execution-plan/v1/schema.json",
      ...compileJsonSchema(FunctionalAPIExecutionPlanV1),
    },
  },
  {
    name: "selection shadow report",
    outputPath: new URL(
      "../generated/selection-shadow-report/v1/selection-shadow-report.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/selection-shadow-report/v1/schema.json",
      ...compileJsonSchema(SelectionShadowReportV1),
    },
  },
  {
    name: "functional API adapter request",
    outputPath: new URL(
      "../generated/functional-api-adapter-request/v1/functional-api-adapter-request.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/functional-api-adapter-request/v1/schema.json",
      ...compileJsonSchema(FunctionalAPIAdapterRequestV1),
    },
  },
  {
    name: "functional API adapter result",
    outputPath: new URL(
      "../generated/functional-api-adapter-result/v1/functional-api-adapter-result.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/functional-api-adapter-result/v1/schema.json",
      ...compileJsonSchema(FunctionalAPIAdapterResultV1),
    },
  },
  {
    name: "execution attempt",
    outputPath: new URL(
      "../generated/execution-attempt/v1/execution-attempt.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/execution-attempt/v1/schema.json",
      ...compileJsonSchema(ExecutionAttemptV1),
    },
  },
  {
    name: "execution manifest",
    outputPath: new URL(
      "../generated/execution-manifest/v1/execution-manifest.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/execution-manifest/v1/schema.json",
      ...compileJsonSchema(ExecutionManifestV1),
    },
  },
  {
    name: "browser capability selection manifest",
    outputPath: new URL(
      "../generated/execution-manifest/v2/execution-manifest.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/execution-manifest/v2/schema.json",
      ...compileJsonSchema(ExecutionManifestV2),
    },
  },
  {
    name: "browser component selection manifest",
    outputPath: new URL(
      "../generated/execution-manifest/v3/execution-manifest.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/execution-manifest/v3/schema.json",
      ...compileJsonSchema(ExecutionManifestV3),
    },
  },
  {
    name: "reviewed functional API selection manifest",
    outputPath: new URL(
      "../generated/execution-manifest/v4/execution-manifest.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/execution-manifest/v4/schema.json",
      ...compileJsonSchema(ExecutionManifestV4),
    },
  },
  {
    name: "reviewed selection policy",
    outputPath: new URL(
      "../generated/reviewed-selection-policy/v1/reviewed-selection-policy.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/reviewed-selection-policy/v1/schema.json",
      ...compileJsonSchema(ReviewedSelectionPolicyV1),
    },
  },
  {
    name: "head test inventory",
    outputPath: new URL(
      "../generated/head-test-inventory/v1/head-test-inventory.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/head-test-inventory/v1/schema.json",
      ...compileJsonSchema(HeadTestInventoryV1),
    },
  },
  {
    name: "capability impact",
    outputPath: new URL(
      "../generated/capability-impact/v1/capability-impact.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/capability-impact/v1/schema.json",
      ...compileJsonSchema(CapabilityImpactV1),
    },
  },
  {
    name: "capability impact v2",
    outputPath: new URL(
      "../generated/capability-impact/v2/capability-impact.schema.json",
      import.meta.url,
    ),
    document: {
      $id: "https://argus.dev/contracts/capability-impact/v2/schema.json",
      ...compileJsonSchema(CapabilityImpactV2),
    },
  },
  {
    name: "change set",
    outputPath: new URL("../generated/change-set/v1/change-set.schema.json", import.meta.url),
    document: {
      $id: "https://argus.dev/contracts/change-set/v1/schema.json",
      ...compileJsonSchema(ChangeSetV1),
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
      ...compileJsonSchema(RepositoryDescriptorV1),
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
      ...compileJsonSchema(TestCatalogPageV1),
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
      ...compileJsonSchema(ImpactEvidenceBundleV1),
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
      ...compileJsonSchema(ImpactEdgePageV1),
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
