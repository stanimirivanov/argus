import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import {
  FieldValidationEvidenceV1,
  FieldValidationRejectionV1,
  FunctionalAPIFieldValidationRequestV1,
  FunctionalAPIFieldValidationResultV1,
} from "./field-validation-v1.js";

const source = new URL(
  "../fixtures/adaptation-proposal/v1/valid/endpoint-rename.json",
  import.meta.url,
);

test("field validation contracts require repeat ordinals and separate endpoint versions", async () => {
  const proposal = JSON.parse(await readFile(source, "utf8"));
  const request = {
    apiVersion: "argus.dev/functional-api-field-validation-request/v1",
    validationId: "b".repeat(64),
    proposalId: proposal.proposalId,
    phase: "original",
    repeat: 1,
    test: proposal.test,
    sourceSha256: "1".repeat(64),
  };
  const decodeRequest = Schema.decodeUnknownResult(FunctionalAPIFieldValidationRequestV1, {
    onExcessProperty: "error",
  });
  assert.equal(decodeRequest(request)._tag, "Success");
  assert.equal(decodeRequest({ ...request, repeat: 3 })._tag, "Failure");
  assert.equal(
    decodeRequest({
      ...request,
      apiVersion: "argus.dev/functional-api-repair-validation-request/v1",
    })._tag,
    "Failure",
  );

  const result = {
    apiVersion: "argus.dev/functional-api-field-validation-result/v1",
    validationId: request.validationId,
    proposalId: request.proposalId,
    phase: "original",
    repeat: 1,
    suiteKey: proposal.test.suiteKey,
    testKey: proposal.test.testKey,
    adapter: proposal.adapter,
    sourceSha256: request.sourceSha256,
    startedAt: "2026-10-17T10:00:00Z",
    completedAt: "2026-10-17T10:00:01Z",
    outcome: "failed",
    failure: { code: "assertion-failed", message: "stale field" },
  };
  const decodeResult = Schema.decodeUnknownResult(FunctionalAPIFieldValidationResultV1, {
    onExcessProperty: "error",
  });
  assert.equal(decodeResult(result)._tag, "Success");
  assert.equal(decodeResult({ ...result, repeat: 0 })._tag, "Failure");
});

test("field success and rejection evidence have distinct wire identities", async () => {
  const proposal = JSON.parse(await readFile(source, "utf8"));
  const edit = {
    path: "tests/fields.spec.mjs",
    beforeSha256: "1".repeat(64),
    startByte: 12,
    endByte: 19,
    original: "orderId",
    replacement: "purchaseId",
    semanticRole: "response-field-access",
  };
  const hashes = ["1".repeat(64), "2".repeat(64), "3".repeat(64)];
  const phases = ["original", "candidate", "negative-control"];
  const runs = phases.flatMap((phase, index) =>
    [1, 2].map((repeat) => ({
      phase,
      repeat,
      sourceSha256: hashes[index],
      startedAt: `2026-10-17T10:00:0${index * 2 + repeat}Z`,
      completedAt: `2026-10-17T10:00:0${index * 2 + repeat}Z`,
      outcome: index === 1 ? "passed" : "failed",
      failure: index === 1 ? null : { code: "assertion-failed", message: "field mismatch" },
    })),
  );
  const evidence = {
    apiVersion: "argus.dev/field-validation-evidence/v1",
    policyVersion: "argus.dev/validation-policy/functional-api-compatible-field-rename/v1",
    validationId: "b".repeat(64),
    proposalId: proposal.proposalId,
    proposalPolicyVersion: "argus.dev/adaptation-policy/functional-api-compatible-field-rename/v1",
    test: proposal.test,
    adapter: proposal.adapter,
    edit,
    source: {
      path: edit.path,
      originalSha256: hashes[0],
      candidateSha256: hashes[1],
      negativeSha256: hashes[2],
      restoredSha256: hashes[0],
      negativeControlField: "__argus_negative_field_aaaaaaaaaaaa",
    },
    runs,
  };
  const decodeEvidence = Schema.decodeUnknownResult(FieldValidationEvidenceV1, {
    onExcessProperty: "error",
  });
  const decodeRejection = Schema.decodeUnknownResult(FieldValidationRejectionV1, {
    onExcessProperty: "error",
  });
  assert.equal(decodeEvidence(evidence)._tag, "Success");
  assert.equal(decodeEvidence({ ...evidence, runs: runs.slice(0, 5) })._tag, "Failure");
  assert.equal(
    decodeEvidence({ ...evidence, apiVersion: "argus.dev/validation-evidence/v1" })._tag,
    "Failure",
  );
  const rejection = {
    ...evidence,
    apiVersion: "argus.dev/field-validation-rejection/v1",
    rejection: {
      phase: "original",
      reason: "original-passed",
      expectedOutcome: "failed",
      actualOutcome: "passed",
    },
    runs: runs.slice(0, 2).map((run) => ({ ...run, outcome: "passed", failure: null })),
  };
  assert.equal(decodeRejection(rejection)._tag, "Success");
  assert.equal(decodeEvidence(rejection)._tag, "Failure");
});
