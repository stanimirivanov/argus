import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";

import {
  FieldAdaptationProposalV1,
  FunctionalAPIFieldAdaptationRequestV1,
  FunctionalAPIFieldAdaptationResultV1,
} from "./field-adaptation-v1.js";

const root = new URL("../", import.meta.url);
const rename = {
  documentPath: "api/orders.yaml",
  baseSha256: "1".repeat(64),
  headSha256: "2".repeat(64),
  method: "POST",
  path: "/orders",
  operationId: "createOrder",
  capabilities: ["create-order"],
  direction: "request",
  statusCode: "",
  previousName: "customerId",
  name: "clientId",
};
const edit = {
  path: "tests/orders.spec.mjs",
  beforeSha256: "3".repeat(64),
  startByte: 42,
  endByte: 52,
  original: "customerId",
  replacement: "clientId",
  semanticRole: "request-body-key",
};

async function endpointFixture(path: string): Promise<Record<string, unknown>> {
  return JSON.parse(await readFile(new URL(path, root), "utf8"));
}

test("field contracts distinguish proposal, request and result from endpoint v1", async () => {
  const decodeRequest = Schema.decodeUnknownResult(FunctionalAPIFieldAdaptationRequestV1, {
    onExcessProperty: "error",
  });
  const decodeResult = Schema.decodeUnknownResult(FunctionalAPIFieldAdaptationResultV1, {
    onExcessProperty: "error",
  });
  const decodeProposal = Schema.decodeUnknownResult(FieldAdaptationProposalV1, {
    onExcessProperty: "error",
  });
  const endpointRequest = await endpointFixture(
    "fixtures/functional-api-adaptation-request/v1/valid/endpoint-rename.json",
  );
  const { endpointRename: _requestRename, ...requestRest } = endpointRequest;
  const request = {
    ...requestRest,
    apiVersion: "argus.dev/functional-api-field-adaptation-request/v1",
    fieldRename: rename,
  };
  assert.equal(decodeRequest(request)._tag, "Success");
  assert.equal(decodeRequest(endpointRequest)._tag, "Failure");

  const endpointResult = await endpointFixture(
    "fixtures/functional-api-adaptation-result/v1/valid/candidate.json",
  );
  const result = {
    ...endpointResult,
    apiVersion: "argus.dev/functional-api-field-adaptation-result/v1",
    edit,
  };
  assert.equal(decodeResult(result)._tag, "Success");
  assert.equal(
    decodeResult({ ...result, edit: { ...edit, semanticRole: "assertion" } })._tag,
    "Failure",
  );

  const endpointProposal = await endpointFixture(
    "fixtures/adaptation-proposal/v1/valid/endpoint-rename.json",
  );
  const { endpointRename: _proposalRename, ...proposalRest } = endpointProposal;
  const proposal = {
    ...proposalRest,
    apiVersion: "argus.dev/field-adaptation-proposal/v1",
    policyVersion: "argus.dev/adaptation-policy/functional-api-compatible-field-rename/v1",
    fieldRename: rename,
    edit,
  };
  assert.equal(decodeProposal(proposal)._tag, "Success");
  assert.equal(decodeProposal(endpointProposal)._tag, "Failure");
});
