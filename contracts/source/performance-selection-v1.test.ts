import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";
import { PerformanceMappingPolicyV1, PerformanceSelectionV1 } from "./performance-selection-v1.js";

const sha = "a".repeat(64);
const ref = { issuer: "https://provider.example", id: "resource", revision: "1", sha256: sha };
const revision = { algorithm: "git-sha1", digest: "a".repeat(40) };
const policy = {
  apiVersion: "argus.dev/performance-mapping-policy/v1",
  repository: { provider: "github", host: "github.com", providerRepositoryId: "repo" },
  headRevision: revision,
  impactSha256: sha,
  catalogSha256: sha,
  reviewId: "review",
  reviewedBy: "owner",
  reviewedAt: "2026-10-22T11:00:00Z",
  expiresAt: "2026-10-22T13:00:00Z",
  headCapabilities: [],
  allowedProfiles: [],
  deployments: [],
  mappings: [],
};

test("performance mapping contract keeps exact review pins and rejects unsafe fields", () => {
  const decode = Schema.decodeUnknownSync(PerformanceMappingPolicyV1);
  assert.equal(decode(policy).impactSha256, policy.impactSha256);
  assert.throws(() => decode({ ...policy, headRevision: { ...revision, digest: "main" } }));
  assert.throws(() => decode({ ...policy, impactSha256: "unknown" }));
  assert.throws(() => decode({ ...policy, deployments: null }));
  // Generated JSON Schema enforces closed objects; Effect excess-key enforcement
  // is verified through its explicit parse option at the same boundary.
  assert.throws(() => decode({ ...policy, execute: true }, { onExcessProperty: "error" }));
});

test("performance recommendation cannot omit owner approval or candidate provenance", async () => {
  const source = JSON.parse(
    await readFile(
      new URL("../fixtures/capability-impact/v1/valid/orders-api.json", import.meta.url),
      "utf8",
    ),
  );
  const {
    apiVersion: _,
    analyzerVersion: __,
    status: ___,
    documents: ____,
    warnings: _____,
    ...change
  } = source;
  const result = {
    apiVersion: "argus.dev/performance-selection/v1",
    policyVersion: "argus.dev/performance-selection-policy/v1",
    change,
    impact: { apiVersion: source.apiVersion, analyzerVersion: source.analyzerVersion, sha256: sha },
    catalog: {
      reference: ref,
      fingerprint: sha,
      schemaSha256: sha,
      scope: { issuer: ref.issuer, principalIssuer: "https://auth.example", principalId: "argus" },
      validUntil: "2026-10-22T13:00:00Z",
    },
    policySha256: sha,
    reviewId: "review",
    evaluatedAt: "2026-10-22T12:00:00Z",
    expiresAt: "2026-10-22T13:00:00Z",
    ownerApprovalRequired: true,
    status: "review-required",
    decisions: [
      {
        mappingId: null,
        capability: null,
        outcome: "review-required",
        reason: "impact-incomplete",
        target: null,
        ownerRoute: null,
        deployment: null,
      },
    ],
  };
  const decode = Schema.decodeUnknownSync(PerformanceSelectionV1);
  assert.equal(decode(result).ownerApprovalRequired, true);
  assert.throws(() => decode({ ...result, ownerApprovalRequired: false }));
  assert.throws(() =>
    decode({
      ...result,
      decisions: [
        { ...result.decisions[0], outcome: "candidate", reason: "owner-approval-required" },
      ],
    }),
  );
});
