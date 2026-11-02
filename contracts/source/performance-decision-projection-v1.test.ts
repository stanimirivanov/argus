import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { Schema } from "effect";
import { compileJsonSchema } from "../scripts/json-schema.js";
import { PerformanceDecisionProjectionV1 } from "./performance-decision-projection-v1.js";

const decode = Schema.decodeUnknownSync(PerformanceDecisionProjectionV1, {
  onExcessProperty: "error",
});
const sha = "a".repeat(64);
const ref = (id: string) => ({ issuer: "https://perfeng.example", id, revision: "1", sha256: sha });
const identity = (id: string) => ({ issuer: "https://argus.example", id });
const observed = "2026-11-02T10:00:00Z";
const dimension = (status: string) => ({
  status,
  reasons: status === "PASS" ? [] : ["fixture-reason"],
});

async function example() {
  const impact = JSON.parse(
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
  } = impact;
  return {
    apiVersion: "argus.dev/performance-decision-projection/v1",
    scope: {
      issuer: "https://perfeng.example",
      principalIssuer: "https://auth.example",
      principalId: "argus",
    },
    provenance: {
      change,
      impactSha256: sha,
      selectionSha256: sha,
      mappingPolicySha256: sha,
      catalogSha256: sha,
      reviewSha256: sha,
      intentFingerprint: sha,
    },
    request: {
      reference: identity("request"),
      attempt: identity("attempt"),
      eventId: "request-event",
      payloadSha256: sha,
      requestedAt: observed,
      deadline: "2026-11-02T10:05:00Z",
    },
    target: {
      service: ref("orders"),
      capability: ref("checkout"),
      deployment: ref("staging"),
      source: change.headRevision,
      imageSha256: sha,
      workload: ref("checkout"),
      profile: ref("smoke"),
      environment: ref("fixture"),
      policy: ref("latency"),
      baseline: null,
    },
    run: { issuer: "https://perfeng.example", id: "run-1" },
    lifecycle: null,
    analysis: {
      conflict: false,
      decisions: [
        {
          eventId: "analysis-event",
          evidenceFingerprint: sha,
          reportSha256: sha,
          publishedAt: observed,
          late: false,
          rules: [
            {
              ruleId: "latency",
              metric: "http_req_duration",
              statistic: "p95",
              unit: "ms",
              quality: dimension("PASS"),
              slo: dimension("FAIL"),
              regression: dimension("NOT_EVALUATED"),
            },
          ],
        },
      ],
    },
  };
}

test("preserves mixed outcomes, source and immutable evidence correlation", async () => {
  const input = await example();
  const output = decode(input);
  assert.equal(output.target.source.digest, input.provenance.change.headRevision.digest);
  assert.equal(output.analysis.decisions[0]?.reportSha256, sha);
  const rule = output.analysis.decisions[0]?.rules[0];
  assert.equal(rule?.quality.status, "PASS");
  assert.equal(rule?.slo.status, "FAIL");
  assert.equal(rule?.regression.status, "NOT_EVALUATED");
  assert.equal(output.lifecycle, null);
});

test("empty known history has no fabricated decision or lifecycle success", async () => {
  const input = await example();
  input.analysis.decisions = [];
  const output = decode(input);
  assert.deepEqual(output.analysis.decisions, []);
  assert.equal(output.analysis.conflict, false);
  assert.throws(() => decode({ ...input, analysis: null }));
  assert.throws(() => decode({ ...input, analysis: { conflict: false } }));
});

test("retains conflicting reports, late publication and independent inconclusive outcomes", async () => {
  const input = await example();
  const first = input.analysis.decisions[0];
  assert.ok(first);
  const originalRule = first.rules[0];
  assert.ok(originalRule);
  first.rules[0] = {
    ...originalRule,
    quality: dimension("UNSTABLE"),
    slo: dimension("INCONCLUSIVE"),
    regression: dimension("INCONCLUSIVE"),
  };
  input.analysis = {
    conflict: true,
    decisions: [
      first,
      {
        ...first,
        eventId: "late-event",
        reportSha256: "b".repeat(64),
        publishedAt: "2026-11-02T10:06:00Z",
        late: true,
      },
    ],
  };
  const output = decode(input);
  assert.equal(output.analysis.decisions.length, 2);
  assert.equal(output.analysis.conflict, true);
  assert.equal(output.analysis.decisions[1]?.late, true);
  assert.equal(output.analysis.decisions[0]?.rules[0]?.quality.status, "UNSTABLE");
});

test("rejects private fields and aggregate verdicts at every nested boundary", async () => {
  const input = await example();
  for (const field of [
    "token",
    "reviewBundle",
    "reportBytes",
    "objectUri",
    "position",
    "overall",
    "approved",
    "winner",
  ]) {
    for (const path of [
      [],
      ["scope"],
      ["provenance"],
      ["request"],
      ["run"],
      ["target", "policy"],
      ["analysis"],
      ["analysis", "decisions", "0"],
      ["analysis", "decisions", "0", "rules", "0", "slo"],
    ]) {
      const changed = structuredClone(input);
      let node: Record<string, unknown> = changed;
      for (const key of path) node = node[key] as Record<string, unknown>;
      node[field] = "private";
      assert.throws(() => decode(changed), `${path.join(".")}.${field}`);
    }
  }
});

test("rejects malformed pins, impossible status families and collection overflow", async () => {
  const input = await example();
  const decision = input.analysis.decisions[0];
  assert.ok(decision);
  for (const patch of [
    { apiVersion: "argus.dev/performance-decision-projection/v2" },
    { run: null },
    { analysis: { conflict: true, decisions: [] } },
    {
      analysis: {
        conflict: false,
        decisions: [
          { ...decision, rules: [{ ...decision.rules[0], quality: dimension("INVALID") }] },
        ],
      },
    },
    { provenance: { ...input.provenance, selectionSha256: "main" } },
    { analysis: { conflict: false, decisions: Array(1001).fill(decision) } },
    { analysis: { conflict: false, decisions: [{ ...decision, eventId: "" }] } },
    { analysis: { conflict: false, decisions: [{ ...decision, publishedAt: "invalid" }] } },
    { analysis: { conflict: false, decisions: [{ ...decision, rules: [] }] } },
    {
      analysis: {
        conflict: false,
        decisions: [{ ...decision, rules: Array(1001).fill(decision.rules[0]) }],
      },
    },
    ...["quality", "slo", "regression"].map((key) => ({
      analysis: {
        conflict: false,
        decisions: [
          {
            ...decision,
            rules: [
              { ...decision.rules[0], [key]: dimension(key === "quality" ? "FAIL" : "INVALID") },
            ],
          },
        ],
      },
    })),
  ])
    assert.throws(() => decode({ ...input, ...patch }));
});

test("lifecycle revisions remain execution observations and baseline pins stay explicit", async () => {
  const input = await example();
  const lifecycle = {
    eventId: "lifecycle-event",
    evidenceFingerprint: sha,
    revision: 4,
    state: "FAILED",
    observedAt: observed,
  };
  const output = decode({
    ...input,
    target: { ...input.target, baseline: ref("baseline") },
    lifecycle,
  });
  assert.equal(output.lifecycle?.state, "FAILED");
  assert.equal(output.target.baseline?.id, "baseline");
  assert.equal(output.analysis.decisions[0]?.rules[0]?.quality.status, "PASS");
  for (const revision of [-1, 0.5, Number.MAX_SAFE_INTEGER + 1]) {
    assert.throws(() => decode({ ...input, lifecycle: { ...lifecycle, revision } }));
  }
  assert.throws(() => decode({ ...input, lifecycle: { ...lifecycle, state: "PASSED" } }));
});

test("portable schema closes all objects and retains digest, event and rule bounds", () => {
  const schema = compileJsonSchema(PerformanceDecisionProjectionV1);
  const visit = (value: unknown): void => {
    if (typeof value !== "object" || value === null) return;
    const object = value as Record<string, unknown>;
    if (object.type === "object") assert.equal(object.additionalProperties, false);
    for (const child of Object.values(object)) visit(child);
  };
  visit(schema);
  const wire = JSON.stringify(schema);
  assert.match(wire, /\^\[0-9a-f\]\{64\}\$/);
  assert.match(wire, /"maxItems":1000/);
  assert.match(wire, /"maximum":9007199254740991/);
});
