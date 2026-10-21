import { spawn } from "node:child_process";
import { cp, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { startFixtureService } from "./fixture-service.mjs";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const fixture = dirname(fileURLToPath(import.meta.url));
const adapter = join(fixture, "playwright-adapter.mjs");
const repairAdapter = join(fixture, "playwright-repair-adapter.mjs");
const sourceIdentity = {
  provider: "github",
  host: "github.com",
  providerRepositoryId: "fixture-source-1",
};
const testRepositories = [
  {
    directory: "test-repository",
    groupKey: "orders-playwright",
    repository: {
      provider: "github",
      host: "github.com",
      providerRepositoryId: "fixture-tests-orders",
      owner: "argus-fixture",
      name: "orders-tests",
    },
  },
  {
    directory: "billing-test-repository",
    groupKey: "billing-playwright",
    repository: {
      provider: "github",
      host: "github.com",
      providerRepositoryId: "fixture-tests-billing",
      owner: "argus-fixture",
      name: "billing-tests",
    },
  },
];

async function command(
  executable,
  args,
  { cwd = root, env = {}, acceptFailure = false, input } = {},
) {
  const pnpmScript = executable === "pnpm" ? process.env.npm_execpath : undefined;
  const child = spawn(
    pnpmScript ? process.execPath : executable,
    pnpmScript ? [pnpmScript, ...args] : args,
    {
      cwd,
      env: { ...process.env, ...env },
      windowsHide: true,
      shell: executable === "pnpm" && !pnpmScript && process.platform === "win32",
    },
  );
  if (input !== undefined) child.stdin.end(input);
  const stdout = [];
  const stderr = [];
  child.stdout.on("data", (chunk) => stdout.push(chunk));
  child.stderr.on("data", (chunk) => stderr.push(chunk));
  const code = await new Promise((done, reject) => {
    child.once("error", reject);
    child.once("close", done);
  });
  const result = {
    code,
    stdout: Buffer.concat(stdout).toString("utf8"),
    stderr: Buffer.concat(stderr).toString("utf8"),
  };
  if (code !== 0 && !acceptFailure)
    throw new Error(
      `${executable} ${args[0]} failed (exit ${code}): ${[result.stdout, result.stderr]
        .filter(Boolean)
        .join("\n")
        .slice(-8192)}`,
    );
  return result;
}

function parse(result, label) {
  try {
    return JSON.parse(result.stdout);
  } catch {
    throw new Error(`${label} did not produce JSON: ${result.stderr}`);
  }
}

async function git(directory, ...args) {
  return (await command("git", args, { cwd: directory })).stdout.trim();
}

async function initialize(directory) {
  await git(directory, "init", "-q");
  await git(directory, "config", "user.email", "argus-fixture@example.invalid");
  await git(directory, "config", "user.name", "Argus fixture");
  await git(directory, "add", ".");
  await git(directory, "commit", "-qm", "Initialize fixture");
  return git(directory, "rev-parse", "HEAD");
}

async function prepareSource(directory) {
  await mkdir(join(directory, "src", "orders"), { recursive: true });
  await writeFile(
    join(directory, "src", "orders", "service.go"),
    "package orders\n\nconst revision = 1\n",
  );
  const base = await initialize(directory);
  await writeFile(
    join(directory, "src", "orders", "service.go"),
    "package orders\n\nconst revision = 2\n",
  );
  await git(directory, "add", ".");
  await git(directory, "commit", "-qm", "Change order service");
  const head = await git(directory, "rev-parse", "HEAD");
  const changed = await git(directory, "diff", "--name-only", base, head);
  if (changed !== "src/orders/service.go")
    throw new Error("source fixture change differs from analyzed path");
  return { base, head };
}

async function prepareTests(temp, entry) {
  const directory = join(temp, entry.directory);
  await cp(join(fixture, entry.directory), directory, {
    recursive: true,
    filter: (path) =>
      !["node_modules", "test-results", "playwright-report", ".pnpm-store"].some(
        (name) => path.includes(`${sep}${name}${sep}`) || path.endsWith(`${sep}${name}`),
      ),
  });
  const offlineInstall = await command("pnpm", ["install", "--frozen-lockfile", "--offline"], {
    cwd: directory,
    acceptFailure: true,
  });
  if (offlineInstall.code !== 0) {
    // A fresh CI store can contain the root importer's packages without the
    // metadata needed to resolve an independent fixture lockfile offline.
    await command("pnpm", ["install", "--frozen-lockfile"], { cwd: directory });
  }
  const revision = await initialize(directory);
  const env = {
    ARGUS_SOURCE_REPOSITORY: JSON.stringify(sourceIdentity),
    ARGUS_TEST_REPOSITORY: JSON.stringify(entry.repository),
    ARGUS_BASE_TEST_REVISION: revision,
  };
  const inventory = parse(
    await command(process.execPath, [adapter, "discover"], { cwd: directory, env }),
    "head inventory",
  );
  if (
    !inventory.complete ||
    inventory.tests.length === 0 ||
    inventory.tests.some((test) => test.new || test.modified || test.revision.digest !== revision)
  ) {
    throw new Error("trusted discovery returned incomplete or inconsistent inventory");
  }
  return { ...entry, directory, revision, inventory };
}

async function build(temp, name, path) {
  const executable = join(temp, process.platform === "win32" ? `${name}.exe` : name);
  await command("go", ["build", "-o", executable, path]);
  return executable;
}

async function expectFailure(executable, args, options, label) {
  const result = await command(executable, args, { ...options, acceptFailure: true });
  if (result.code === 0 || result.stdout.trim()) throw new Error(`${label} did not fail closed`);
}

async function campaign(temp, service) {
  const source = await prepareSource(join(temp, "source-repository"));
  const tests = [];
  for (const entry of testRepositories) tests.push(await prepareTests(temp, entry));
  const inventory = {
    apiVersion: "argus.dev/head-test-inventory/v1",
    sourceRepository: sourceIdentity,
    complete: true,
    tests: tests.flatMap((entry) => entry.inventory.tests),
  };
  const policy = {
    apiVersion: "argus.dev/reviewed-selection-policy/v1",
    sourceRepository: sourceIdentity,
    mustRunCapabilities: [],
    mustRunTests: [],
    pins: [],
    exclusions: [],
  };
  const policyPath = join(temp, "policy.json");
  const inventoryPath = join(temp, "inventory.json");
  const manifestPath = join(temp, "manifest.json");
  const bindingsPath = join(temp, "bindings.json");
  const planPath = join(temp, "plan.json");
  const attemptDirectory = join(temp, "attempts");
  await mkdir(attemptDirectory);
  await writeFile(policyPath, JSON.stringify(policy));
  await writeFile(inventoryPath, JSON.stringify(inventory));
  const select = await build(
    temp,
    "select-reference",
    "./examples/functional-api/select-reference",
  );
  const planCommand = await build(temp, "plan-functional-api", "./cmd/plan-functional-api");
  const runner = await build(temp, "run-functional-api", "./cmd/run-functional-api");
  const reportCommand = await build(
    temp,
    "report-reference",
    "./examples/functional-api/report-reference",
  );
  const selectArgs = [
    "-source-base",
    source.base,
    "-source-head",
    source.head,
    "-policy",
    policyPath,
    "-inventory",
    inventoryPath,
  ];
  const manifest = parse(await command(select, selectArgs), "reviewed manifest");
  if (
    manifest.apiVersion !== "argus.dev/execution-manifest/v4" ||
    manifest.mode !== "targeted" ||
    manifest.decisions.filter((decision) => decision.outcome === "RUN_REQUIRED").length !== 1
  ) {
    throw new Error("source change did not select only the impacted test");
  }
  await writeFile(manifestPath, JSON.stringify(manifest));
  const bindings = {
    apiVersion: "argus.dev/functional-api-execution-bindings/v1",
    groups: tests.map((entry) => ({
      groupKey: entry.groupKey,
      testRepository: entry.repository,
      testRevision: { algorithm: "git-sha1", digest: entry.revision },
      adapter: "playwright",
    })),
  };
  await writeFile(bindingsPath, JSON.stringify(bindings));
  const plan = parse(
    await command(planCommand, ["-manifest", manifestPath, "-bindings", bindingsPath]),
    "execution plan",
  );
  if (
    plan.jobs.length !== 3 ||
    plan.jobs.filter((job) => job.stage === "full-suite").length !== 2
  ) {
    throw new Error("plan omitted a selected or full-only control group");
  }
  await writeFile(planPath, JSON.stringify(plan));
  const probe = tests[0];
  const stagedProbe = {
    apiVersion: "argus.dev/functional-api-adapter-request/v2",
    attemptId: "staged-protocol-probe",
    manifest: plan.manifest,
    stage: "shadow-control",
    testRepository: probe.repository,
    testRevision: { algorithm: "git-sha1", digest: probe.revision },
    adapter: "playwright",
    staging: {
      schedule: { apiVersion: "argus.dev/predictive-portfolio/v1", sha256: "a".repeat(64) },
      episodeId: "probe",
      groupKey: probe.groupKey,
      environment: "ci",
      mode: "shadow",
    },
    tests: manifest.decisions
      .filter(
        (d) => d.testRepository.providerRepositoryId === probe.repository.providerRepositoryId,
      )
      .map((d) => ({ suiteKey: d.suite.key, testKey: d.test.key, name: d.test.name })),
  };
  const probeResult = parse(
    await command(process.execPath, [adapter, "execute"], {
      cwd: probe.directory,
      env: { ARGUS_FIXTURE_URL: service.url },
      input: JSON.stringify(stagedProbe),
    }),
    "staged adapter probe",
  );
  if (
    probeResult.attemptId !== stagedProbe.attemptId ||
    probeResult.results.length !== stagedProbe.tests.length
  )
    throw new Error("staged adapter probe lost exact tests");
  const attemptBindings = new Map();
  const attemptFiles = [];
  for (const job of plan.jobs) {
    const entry = tests.find((candidate) => candidate.groupKey === job.groupKey);
    const attemptID = `reference-${job.groupKey}-${job.stage}`;
    const args = [
      "-manifest",
      manifestPath,
      "-stage",
      job.stage,
      "-attempt-id",
      attemptID,
      "-test-provider",
      "github",
      "-test-host",
      "github.com",
      "-test-repository-id",
      entry.repository.providerRepositoryId,
      "-test-revision",
      entry.revision,
      "-adapter",
      "playwright",
      "--",
      process.execPath,
      adapter,
      "execute",
    ];
    const result = await command(runner, args, {
      cwd: entry.directory,
      env: { ARGUS_FIXTURE_URL: service.url },
      acceptFailure: true,
    });
    const attempt = parse(result, `attempt ${attemptID}`);
    if (
      attempt.attemptId !== attemptID ||
      attempt.manifest.apiVersion !== manifest.apiVersion ||
      (result.code !== 0) !== (attempt.outcome !== "passed")
    ) {
      throw new Error(`attempt ${attemptID} has incorrect status or provenance`);
    }
    const file = join(attemptDirectory, `${attemptID}.json`);
    await writeFile(file, JSON.stringify(attempt));
    attemptFiles.push(file);
    const group = attemptBindings.get(job.groupKey) ?? {
      groupKey: job.groupKey,
      selectedAttemptId: null,
      fullSuiteAttemptId: "",
    };
    if (job.stage === "selected") group.selectedAttemptId = attemptID;
    else group.fullSuiteAttemptId = attemptID;
    attemptBindings.set(job.groupKey, group);
  }
  const attemptsPath = join(temp, "attempt-bindings.json");
  await writeFile(
    attemptsPath,
    JSON.stringify({
      apiVersion: "argus.dev/execution-plan-attempt-bindings/v1",
      groups: [...attemptBindings.values()],
    }),
  );
  const report = parse(
    await command(reportCommand, [
      "-plan",
      planPath,
      "-attempt-bindings",
      attemptsPath,
      "-attempt-directory",
      attemptDirectory,
    ]),
    "shadow report",
  );
  if (
    report.fullOnlyGroupCount !== 1 ||
    report.fullSuiteFailureCount !== 1 ||
    report.caughtFullSuiteFailureCount !== 0 ||
    report.groups.length !== 2 ||
    !report.groups.some((group) =>
      group.missedFailures.some((miss) => miss.reason === "not-selected"),
    )
  ) {
    throw new Error("complete plan did not expose the deliberately missed failure");
  }
  await writeFile(join(temp, "shadow-report.json"), JSON.stringify(report));
  const incompletePath = join(temp, "incomplete-inventory.json");
  await writeFile(incompletePath, JSON.stringify({ ...inventory, complete: false }));
  await expectFailure(
    select,
    [...selectArgs.slice(0, -1), incompletePath],
    {},
    "incomplete head inventory",
  );
  const orderTests = tests.find((entry) => entry.groupKey === "orders-playwright");
  const mismatchArgs = [
    "-manifest",
    manifestPath,
    "-stage",
    "selected",
    "-attempt-id",
    "revision-mismatch",
    "-test-repository-id",
    orderTests.repository.providerRepositoryId,
    "-test-revision",
    "0".repeat(40),
    "-adapter",
    "playwright",
    "--",
    process.execPath,
    adapter,
    "execute",
  ];
  await expectFailure(
    runner,
    mismatchArgs,
    { cwd: orderTests.directory, env: { ARGUS_FIXTURE_URL: service.url } },
    "test revision mismatch",
  );
  const malformedArgs = [
    "-manifest",
    manifestPath,
    "-stage",
    "selected",
    "-attempt-id",
    "malformed-output",
    "-test-repository-id",
    orderTests.repository.providerRepositoryId,
    "-test-revision",
    orderTests.revision,
    "-adapter",
    "playwright",
    "--",
    process.execPath,
    "-e",
    "process.stdout.write('{')",
  ];
  await expectFailure(
    runner,
    malformedArgs,
    { cwd: orderTests.directory },
    "malformed adapter output",
  );
  const mapPath = join(orderTests.directory, "argus-tests.json");
  const changedMap = JSON.parse(await readFile(mapPath, "utf8"));
  changedMap.tests.pop();
  await writeFile(mapPath, JSON.stringify(changedMap));
  await expectFailure(
    process.execPath,
    [adapter, "discover"],
    {
      cwd: orderTests.directory,
      env: {
        ARGUS_SOURCE_REPOSITORY: JSON.stringify(sourceIdentity),
        ARGUS_TEST_REPOSITORY: JSON.stringify(orderTests.repository),
      },
    },
    "incomplete discovery",
  );
  if (process.env.ARGUS_DATABASE_URL) {
    const migrate = await build(temp, "migrate", "./cmd/migrate");
    const evidence = await build(temp, "execution-evidence", "./cmd/execution-evidence");
    await command(migrate, []);
    for (const file of attemptFiles) await command(evidence, ["ingest", "-file", file]);
    const persisted = parse(
      await command(evidence, [
        "plan-shadow-report",
        "-plan",
        planPath,
        "-attempt-bindings",
        attemptsPath,
      ]),
      "persisted shadow report",
    );
    if (
      persisted.fullSuiteFailureCount !== report.fullSuiteFailureCount ||
      persisted.fullOnlyGroupCount !== report.fullOnlyGroupCount
    ) {
      throw new Error("PostgreSQL report differs from file-backed evidence");
    }
  }
  process.stdout.write(
    "api-validate: reviewed selection, Playwright execution, missed failure and recovery checks passed\n",
  );
}

async function prepareRepairSource(directory) {
  const path = join(directory, "api", "openapi.yaml");
  await mkdir(dirname(path), { recursive: true });
  const document = (route) =>
    `openapi: 3.0.3\ninfo:\n  title: Orders API\n  version: 1.0.0\npaths:\n  ${route}:\n    get:\n      operationId: listOrders\n      x-argus-capabilities: [list-orders]\n      responses:\n        '200':\n          description: Orders\n`;
  await writeFile(path, document("/v1/orders"));
  const base = await initialize(directory);
  await writeFile(path, document("/v2/orders"));
  await git(directory, "add", ".");
  await git(directory, "commit", "-qm", "Rename orders endpoint");
  return { base, head: await git(directory, "rev-parse", "HEAD") };
}

async function repairCampaign(temp, service) {
  const source = await prepareRepairSource(join(temp, "repair-source"));
  const test = await prepareTests(temp, {
    directory: "repair-test-repository",
    groupKey: "repair-playwright",
    repository: {
      provider: "github",
      host: "github.com",
      providerRepositoryId: "fixture-tests-repair",
      owner: "argus-fixture",
      name: "repair-tests",
    },
  });
  const inventoryPath = join(temp, "repair-inventory.json");
  const policyPath = join(temp, "repair-policy.json");
  const manifestPath = join(temp, "repair-manifest.json");
  const impactPath = join(temp, "repair-impact.json");
  const proposalPath = join(temp, "repair-proposal.json");
  await writeFile(inventoryPath, JSON.stringify(test.inventory));
  await writeFile(
    policyPath,
    JSON.stringify({
      apiVersion: "argus.dev/reviewed-selection-policy/v1",
      sourceRepository: sourceIdentity,
      mustRunCapabilities: [],
      mustRunTests: [],
      pins: [],
      exclusions: [],
    }),
  );
  const select = await build(temp, "select-repair", "./examples/functional-api/select-reference");
  await readFile(policyPath);
  await readFile(inventoryPath);
  const manifest = parse(
    await command(select, [
      "-source-base",
      source.base,
      "-source-head",
      source.head,
      "-policy",
      policyPath,
      "-inventory",
      inventoryPath,
      "-scenario",
      "endpoint-repair",
      "-source-directory",
      join(temp, "repair-source"),
      "-impact-output",
      impactPath,
    ]),
    "repair manifest",
  );
  if (
    manifest.mode !== "targeted" ||
    manifest.decisions.length !== 1 ||
    manifest.decisions[0].outcome !== "RUN_REQUIRED"
  )
    throw new Error("endpoint repair was not selected");
  await writeFile(manifestPath, JSON.stringify(manifest));
  const runner = await build(temp, "run-repair", "./cmd/run-functional-api");
  const attempts = join(temp, "repair-attempts");
  await mkdir(attempts);
  const originalAttempt = parse(
    await command(
      runner,
      [
        "-manifest",
        manifestPath,
        "-stage",
        "selected",
        "-attempt-id",
        "repair-original",
        "-test-repository-id",
        test.repository.providerRepositoryId,
        "-test-revision",
        test.revision,
        "-adapter",
        "playwright",
        "--",
        process.execPath,
        adapter,
        "execute",
      ],
      { cwd: test.directory, env: { ARGUS_FIXTURE_URL: service.url }, acceptFailure: true },
    ),
    "original repair attempt",
  );
  if (originalAttempt.outcome !== "failed") throw new Error("old endpoint did not fail");
  await writeFile(join(attempts, "original.json"), JSON.stringify(originalAttempt));
  const assess = await build(temp, "assess-repair", "./cmd/assess-functional-api-maintenance");
  const assessArgs = [
    "-impact",
    impactPath,
    "-manifest",
    manifestPath,
    "-inventory",
    inventoryPath,
    "-attempt-directory",
    attempts,
  ];
  const assessment = parse(await command(assess, assessArgs), "maintenance assessment");
  if (assessment.items.length !== 1 || assessment.items[0].state !== "INVALIDATED")
    throw new Error("maintenance assessment did not retain failed rename evidence");
  const wrongRevisionPath = join(attempts, "wrong-revision.json");
  const wrongRevision = structuredClone(originalAttempt);
  wrongRevision.testRevision.digest = "0".repeat(40);
  await writeFile(wrongRevisionPath, JSON.stringify(wrongRevision));
  try {
    await expectFailure(assess, assessArgs, {}, "unreviewed test revision");
  } finally {
    await rm(wrongRevisionPath);
  }
  const propose = await build(temp, "propose-repair", "./cmd/propose-functional-api-repair");
  const proposal = parse(
    await command(
      propose,
      [
        "-impact",
        impactPath,
        "-manifest",
        manifestPath,
        "-test-repository-id",
        test.repository.providerRepositoryId,
        "-suite-key",
        "orders-api",
        "-test-key",
        "renamed-orders-test",
        "-test-revision",
        test.revision,
        "--",
        process.execPath,
        repairAdapter,
        "propose",
      ],
      { cwd: test.directory },
    ),
    "endpoint proposal",
  );
  if (
    proposal.classification !== "INVALIDATED" ||
    proposal.edit?.original !== "/v1/orders" ||
    proposal.edit?.replacement !== "/v2/orders"
  )
    throw new Error("unsafe endpoint proposal");
  await writeFile(proposalPath, JSON.stringify(proposal));
  const validate = await build(temp, "validate-repair", "./cmd/validate-functional-api-repair");
  const original = await readFile(join(test.directory, proposal.edit.path));
  const evidence = parse(
    await command(
      validate,
      [
        "-proposal",
        proposalPath,
        "-disposable-workspace",
        test.directory,
        "--",
        process.execPath,
        repairAdapter,
        "validate",
      ],
      { cwd: test.directory, env: { ARGUS_FIXTURE_URL: service.url } },
    ),
    "repair validation",
  );
  if (
    evidence.runs?.map((run) => run.outcome).join(",") !== "failed,passed,failed" ||
    !original.equals(await readFile(join(test.directory, proposal.edit.path)))
  )
    throw new Error("repair validation did not prove all phases and restoration");
  const sourcePath = join(test.directory, proposal.edit.path);
  const proposeArgs = [
    "-impact",
    impactPath,
    "-manifest",
    manifestPath,
    "-test-repository-id",
    test.repository.providerRepositoryId,
    "-suite-key",
    "orders-api",
    "-test-key",
    "renamed-orders-test",
    "-test-revision",
    test.revision,
    "--",
    process.execPath,
    repairAdapter,
    "propose",
  ];
  try {
    await writeFile(
      sourcePath,
      original.toString("utf8").replace('request.get("/v1/orders")', "request.get(dynamicPath)"),
    );
    await expectFailure(propose, proposeArgs, { cwd: test.directory }, "dynamic endpoint syntax");
    await writeFile(
      sourcePath,
      original
        .toString("utf8")
        .replace(
          'const response = await request.get("/v1/orders");',
          'await request.get("/v1/orders");\n  const response = await request.get("/v1/orders");',
        ),
    );
    await expectFailure(propose, proposeArgs, { cwd: test.directory }, "ambiguous endpoint syntax");
  } finally {
    await writeFile(sourcePath, original);
  }
  await command("go", [
    "test",
    "-count=1",
    "-run",
    "TestPublishCreatesDraftAndExactRetryReturnsIt|TestPublishCreatesCorrelatedDraftRequest|TestObserveOutcomeCapturesCompleteReviewerDiff|TestObserveOutcomeRejectsOpenReview|TestCaptureDerivesTerminalReviewDecisions",
    "./internal/adaptation/adapters/githubreview",
    "./internal/adaptation/review",
    "./internal/adaptation/outcome",
  ]);
  process.stdout.write(
    "api-validate: endpoint repair, restoration, and mock GitHub review checks passed\n",
  );
}

const temp = await mkdtemp(join(fixture, ".run-"));
if (!resolve(temp).startsWith(`${resolve(fixture)}${sep}`))
  throw new Error("unsafe fixture cleanup path");
let service;
try {
  service = await startFixtureService({ missedFailure: true });
  if (process.env.ARGUS_REFERENCE_SCENARIO !== "repair") await campaign(temp, service);
  await repairCampaign(temp, service);
} finally {
  if (service) await service.close();
  await rm(temp, { recursive: true, force: true });
}
