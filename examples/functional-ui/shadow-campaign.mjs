// Reference CI campaign: plan, run every job, retain attempts, and compare misses.
import { spawnSync } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import { resolve } from "node:path";

const root = resolve(import.meta.dirname, "../..");
const output = resolve(process.env.ARGUS_BROWSER_CAMPAIGN_DIR ?? ".local/browser-shadow");
await mkdir(output, { recursive: true });
const invoke = (command, args, env = process.env, allowFailure = false) => {
  const result = spawnSync(command, args, {
    cwd: root,
    env,
    encoding: "utf8",
    maxBuffer: 16 << 20,
  });
  if (
    result.error ||
    (!allowFailure && result.status !== 0) ||
    (allowFailure && ![0, 1].includes(result.status))
  )
    throw new Error(`${command} ${args.join(" ")} failed: ${result.error ?? result.stderr}`);
  return result;
};
const revision = invoke("git", ["rev-parse", "HEAD"]).stdout.trim();
const repo = {
  provider: "github",
  host: "github.com",
  providerRepositoryId: "tests-1",
  owner: "example",
  name: "browser-tests",
};
const source = {
  provider: "github",
  host: "github.com",
  providerRepositoryId: "source-1",
  owner: "example",
  name: "shop",
};
const gitRevision = { algorithm: "git-sha1", digest: revision };
const now = "2026-10-18T10:00:00Z";
const manifest = {
  apiVersion: "argus.dev/browser-selection-manifest/v1",
  policyVersion: "argus.dev/selection-policy/browser-mapped/v1",
  change: {
    sourceRepository: source,
    pullRequest: { number: 1 },
    baseRevision: gitRevision,
    headRevision: gitRevision,
    observedAt: now,
    trigger: {
      provider: "github",
      deliveryId: "reference-browser-shadow",
      event: "pull_request",
      action: "synchronize",
    },
  },
  changedPaths: ["ui/shop.ts"],
  pathsComplete: true,
  catalog: {
    sourceRepository: source,
    revision: gitRevision,
    descriptorApiVersion: "argus.dev/repository-descriptor/v1",
  },
  evidence: {
    observationId: "reference-browser-shadow",
    digest: "a".repeat(64),
    collectorVersion: "reference-1",
    mappingRevision: gitRevision,
  },
  evaluatedAt: now,
  family: "functional-ui",
  mode: "targeted",
  warnings: [],
  decisions: [
    {
      testRepository: repo,
      suiteKey: "shop",
      testKey: "checkout",
      name: "checkout shows total [argus:shop:checkout]",
      revision: gitRevision,
      projects: ["chromium-desktop"],
      outcome: "RUN_REQUIRED",
      remainingExecution: "none",
      reason: "DIRECT_UI_MAPPING",
      mappingKeys: ["checkout"],
    },
    {
      testRepository: repo,
      suiteKey: "shop",
      testKey: "receipt",
      name: "receipt shows order [argus:shop:receipt]",
      revision: gitRevision,
      projects: ["chromium-desktop", "chromium-mobile"],
      outcome: "SKIP_FOR_NOW",
      remainingExecution: "full-suite",
      reason: "NOT_AFFECTED_IN_EARLY_STAGE",
      mappingKeys: [],
    },
  ],
};
const bindings = {
  apiVersion: "argus.dev/browser-execution-bindings/v1",
  groups: [
    {
      groupKey: "desktop-ci",
      testRepository: repo,
      testRevision: gitRevision,
      adapter: "playwright",
      project: "chromium-desktop",
      environment: "ci",
    },
    {
      groupKey: "mobile-ci",
      testRepository: repo,
      testRevision: gitRevision,
      adapter: "playwright",
      project: "chromium-mobile",
      environment: "ci",
    },
  ],
};
const save = async (name, value) => {
  const path = resolve(output, name);
  await writeFile(path, `${JSON.stringify(value, null, 2)}\n`);
  return path;
};
const manifestPath = await save("manifest.json", manifest);
const bindingsPath = await save("bindings.json", bindings);
const planRun = invoke("go", [
  "run",
  "./cmd/plan-browser",
  "-manifest",
  manifestPath,
  "-bindings",
  bindingsPath,
]);
const plan = JSON.parse(planRun.stdout);
const planPath = await save("plan.json", plan);
if (process.env.ARGUS_BROWSER_CAMPAIGN_PLAN_ONLY === "1") {
  console.log(`Browser plan: ${plan.jobs.length} jobs in ${planPath}`);
  process.exit(0);
}
const groupAttempts = new Map();
const jobErrors = [];
const runId = `${Date.now()}-${process.pid}`;
for (const job of plan.jobs) {
  const attemptId = `${runId}-${job.groupKey}-${job.stage}`;
  const env = {
    ...process.env,
    ARGUS_BROWSER_TEST_DIR: resolve(root, "examples/functional-ui/test-repository"),
    ARGUS_BROWSER_ARTIFACT_DIR: resolve(output, "artifacts"),
    ARGUS_BROWSER_ENVIRONMENT: job.environment,
    ARGUS_BROWSER_FAIL_RECEIPT: "1",
  };
  try {
    const run = invoke(
      "go",
      [
        "run",
        "./cmd/run-browser",
        "-manifest",
        manifestPath,
        "-attempt-id",
        attemptId,
        "-stage",
        job.stage,
        "-test-repository-id",
        job.testRepository.providerRepositoryId,
        "-test-revision",
        revision,
        "-adapter",
        job.adapter,
        "-project",
        job.project,
        "-environment",
        job.environment,
        "--",
        "node",
        "examples/functional-ui/playwright-adapter.mjs",
      ],
      env,
      true,
    );
    if (!run.stdout.trim())
      throw new Error(`browser ${attemptId} produced no attempt: ${run.stderr}`);
    const attempt = JSON.parse(run.stdout);
    const attemptPath = await save(`${attemptId}.json`, attempt);
    // A failed full suite still has authoritative evidence and must be ingested.
    invoke("go", ["run", "./cmd/execution-evidence", "browser-ingest", "-file", attemptPath], env);
    const group = groupAttempts.get(job.groupKey) ?? {
      groupKey: job.groupKey,
      selectedAttemptId: null,
      fullSuiteAttemptId: "",
    };
    if (job.stage === "selected") group.selectedAttemptId = attemptId;
    else group.fullSuiteAttemptId = attemptId;
    groupAttempts.set(job.groupKey, group);
  } catch (error) {
    jobErrors.push({ groupKey: job.groupKey, stage: job.stage, error: String(error) });
  }
}
if (jobErrors.length) {
  await save("job-errors.json", jobErrors);
  throw new Error(`${jobErrors.length} browser job(s) failed to produce and ingest valid evidence`);
}
const attemptBindingsPath = await save("attempt-bindings.json", {
  apiVersion: "argus.dev/browser-plan-attempt-bindings/v1",
  groups: [...groupAttempts.values()],
});
const compare = invoke("go", [
  "run",
  "./cmd/execution-evidence",
  "browser-plan-shadow-report",
  "-plan",
  planPath,
  "-attempt-bindings",
  attemptBindingsPath,
]);
const report = JSON.parse(compare.stdout);
await save("shadow-report.json", report);
if (
  report.groupCount !== 2 ||
  report.fullOnlyGroupCount !== 1 ||
  report.fullSuiteFailureCount !== 2 ||
  report.caughtFullSuiteFailureCount !== 0 ||
  report.groups.some((group) => group.missedFailures.length !== 1)
)
  throw new Error("reference campaign did not surface both known browser selection misses");
console.log(
  `Browser shadow campaign: ${report.groupCount} groups, ${report.fullSuiteFailureCount} known misses; evidence in ${output}`,
);
