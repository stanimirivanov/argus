// Reference browser adapter. Invoke from a reviewed Playwright test checkout.
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { copyFile, mkdir, mkdtemp, readFile, realpath, rm } from "node:fs/promises";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { isAbsolute, join, resolve, sep } from "node:path";
import { pathToFileURL } from "node:url";

const cwd = resolve(process.env.ARGUS_BROWSER_TEST_DIR ?? process.cwd());
const require = createRequire(resolve(cwd, "package.json"));
const playwrightCLI = require.resolve("@playwright/test/cli");
const version = require("@playwright/test/package.json").version;
const artifactRoot = process.env.ARGUS_BROWSER_ARTIFACT_DIR;
const maxOutput = 8 << 20;
let activeChild;

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    activeChild?.kill(signal);
    process.exitCode = 1;
  });
}

function fail(message) {
  throw new Error(message);
}
async function run(command, args) {
  const child = spawn(command, args, { cwd, windowsHide: true });
  activeChild = child;
  const chunks = [];
  const errors = [];
  let bytes = 0;
  child.stdout.on("data", (chunk) => {
    bytes += chunk.length;
    if (bytes > maxOutput) child.kill();
    else chunks.push(chunk);
  });
  child.stderr.on("data", (chunk) => {
    if (Buffer.concat(errors).length < 64 << 10) errors.push(chunk);
  });
  const code = await new Promise((done, reject) => {
    child.once("error", reject);
    child.once("close", done);
  });
  activeChild = undefined;
  if (bytes > maxOutput) fail("Playwright report exceeded 8 MiB");
  return {
    code,
    stdout: Buffer.concat(chunks).toString("utf8"),
    stderr: Buffer.concat(errors).toString("utf8"),
  };
}
function parse(data, label) {
  try {
    return JSON.parse(data);
  } catch {
    fail(`malformed ${label}`);
  }
}
function flatten(suites) {
  const specs = [];
  for (const suite of suites ?? []) {
    specs.push(...(suite.specs ?? []), ...flatten(suite.suites ?? []));
  }
  return specs;
}
async function gitHead() {
  const result = await run("git", ["rev-parse", "HEAD"]);
  if (result.code !== 0) fail("test checkout revision unavailable");
  return result.stdout.trim();
}
async function requestedRegistry() {
  const map = parse(await readFile(resolve(cwd, "argus-tests.json"), "utf8"), "test registry");
  if (map.apiVersion !== "argus.dev/reference-browser-test-map/v1" || !Array.isArray(map.tests))
    fail("invalid test registry");
  const tests = new Map();
  for (const test of map.tests) {
    const key = `${test.suiteKey}/${test.testKey}`;
    if (
      tests.has(key) ||
      !test.file.startsWith("tests/") ||
      !test.name.includes(`[argus:${test.suiteKey}:${test.testKey}]`)
    )
      fail("ambiguous test registry");
    tests.set(key, test);
  }
  return tests;
}
async function oneTest(request, test, registry, root) {
  const key = `${test.suiteKey}/${test.testKey}`;
  const registered = registry.get(key);
  if (!registered || registered.name !== test.name)
    fail(`test ${key} is absent from reviewed registry`);
  const output = join(root, `${test.suiteKey}-${test.testKey}`);
  const grep = `\\[argus:${test.suiteKey}:${test.testKey}\\]`;
  const runResult = await run(process.execPath, [
    playwrightCLI,
    "test",
    "--project",
    request.project,
    "--reporter=json",
    "--workers=1",
    "--retries=0",
    "--output",
    output,
    "--grep",
    grep,
  ]);
  const report = parse(runResult.stdout, "Playwright result");
  const specs = flatten(report.suites);
  if (
    report.errors?.length ||
    specs.length !== 1 ||
    specs[0].title !== test.name ||
    specs[0].tests?.length !== 1 ||
    specs[0].tests[0].projectName !== request.project ||
    specs[0].tests[0].results?.length !== 1
  )
    fail(`Playwright result mismatch for ${key}: ${runResult.stderr}`);
  const result = specs[0].tests[0].results[0];
  const outcome =
    result.status === "passed"
      ? "passed"
      : result.status === "failed"
        ? "failed"
        : result.status === "skipped"
          ? "skipped"
          : "error";
  if ((runResult.code === 0) !== (outcome === "passed" || outcome === "skipped"))
    fail("Playwright exit/result mismatch");
  const failureMessage = String(result.error?.message ?? `Playwright ${result.status}`);
  const locatorNotFound =
    outcome === "failed" &&
    failureMessage.includes("element(s) not found") &&
    /getBy(?:Role|TestId)\(/u.test(failureMessage);
  const failure =
    outcome === "failed" || outcome === "error"
      ? {
          code: locatorNotFound
            ? "locator-not-found"
            : outcome === "failed"
              ? "assertion"
              : "execution",
          message: failureMessage.slice(0, 2000),
        }
      : null;
  const normalized = {
    suiteKey: test.suiteKey,
    testKey: test.testKey,
    outcome,
    durationMs: Math.max(0, Math.round(result.duration)),
    failure,
  };
  if (outcome === "skipped") return { normalized, artifacts: [] };
  const artifacts = [];
  for (const [kind, name] of [
    ["trace", "trace"],
    ["screenshot", "screenshot"],
    ["accessibility", "argus-accessibility"],
  ]) {
    const attachment = result.attachments?.find((item) => item.name === name && item.path);
    if (!attachment)
      fail(
        `missing ${kind} artifact for ${key}; status: ${result.status}; error: ${result.error?.message}; attachments: ${(result.attachments ?? []).map((item) => item.name).join(",")}`,
      );
    const path = await realpath(attachment.path);
    const verifiedRoot = await realpath(root);
    if (!path.startsWith(`${verifiedRoot}${sep}`))
      fail("Playwright attachment escaped its output directory");
    const bytes = await readFile(path);
    const sha256 = createHash("sha256").update(bytes).digest("hex");
    const artifactKey = `a${createHash("sha256").update(`${key}:${kind}`).digest("hex").slice(0, 16)}-${kind}`;
    const target = resolve(
      artifactRoot,
      request.attemptId,
      `${artifactKey}-${sha256.slice(0, 12)}`,
    );
    await mkdir(resolve(artifactRoot, request.attemptId), { recursive: true });
    await copyFile(path, target);
    artifacts.push({
      key: artifactKey,
      suiteKey: test.suiteKey,
      testKey: test.testKey,
      kind,
      uri: pathToFileURL(target).href,
      sha256,
    });
  }
  return { normalized, artifacts };
}

async function verifyStagedInventory(request, registry) {
  const provenance = request.staging;
  const phases =
    provenance?.mode === "shadow"
      ? ["relevant-now", "shadow-control"]
      : provenance?.mode === "staged"
        ? ["relevant-now", "remaining-later"]
        : [];
  const digest = /^[0-9a-f]{64}$/u;
  const localKey = /^[a-z][a-z0-9._-]{0,62}$/u;
  if (
    !phases.includes(request.stage) ||
    provenance?.environment !== request.environment ||
    provenance?.schedule?.apiVersion !== "argus.dev/predictive-portfolio/v1" ||
    !digest.test(provenance?.schedule?.sha256 ?? "") ||
    !digest.test(provenance?.discoverySha256 ?? "") ||
    !localKey.test(provenance?.episodeId ?? "") ||
    !localKey.test(provenance?.groupKey ?? "") ||
    !Array.isArray(request.candidates) ||
    request.candidates.length === 0 ||
    request.candidates.length > 10000
  )
    fail("invalid staged browser provenance");
  const expected = new Map();
  const byName = new Map();
  for (const test of request.candidates) {
    const key = `${test.suiteKey}/${test.testKey}`;
    if (expected.has(key) || byName.has(test.name) || registry.get(key)?.name !== test.name)
      fail("invalid complete staged inventory");
    expected.set(key, test.name);
    byName.set(test.name, key);
  }
  const listed = await run(process.execPath, [
    playwrightCLI,
    "test",
    "--project",
    request.project,
    "--list",
    "--reporter=json",
  ]);
  if (listed.code !== 0) fail("complete browser discovery failed");
  const report = parse(listed.stdout, "complete browser discovery");
  const specs = flatten(report.suites);
  if (report.errors?.length || specs.length !== expected.size)
    fail("incomplete staged discovery partition");
  const observed = new Set();
  for (const spec of specs) {
    const key = byName.get(spec.title);
    if (
      !key ||
      observed.has(key) ||
      spec.tests?.length !== 1 ||
      spec.tests[0].projectName !== request.project ||
      spec.tests[0].results?.length
    )
      fail("browser discovery differs from complete stage inventory");
    observed.add(key);
  }
  const requested = new Set();
  for (const test of request.tests) {
    const key = `${test.suiteKey}/${test.testKey}`;
    if (requested.has(key) || expected.get(key) !== test.name)
      fail("requested test outside complete inventory");
    requested.add(key);
  }
  if (request.stage === "shadow-control" && request.tests.length !== expected.size)
    fail("incomplete shadow control");
}

async function main() {
  if (!artifactRoot || !isAbsolute(artifactRoot)) fail("absolute artifact directory is required");
  let input = "";
  for await (const chunk of process.stdin) {
    input += chunk;
    if (input.length > 16 << 20) fail("request too large");
  }
  const request = parse(input, "browser adapter request");
  if (
    !["argus.dev/browser-adapter-request/v1", "argus.dev/browser-adapter-request/v2"].includes(
      request.apiVersion,
    ) ||
    request.adapter !== "playwright" ||
    !Array.isArray(request.tests) ||
    request.tests.length === 0 ||
    !request.project ||
    !request.environment ||
    request.environment !== process.env.ARGUS_BROWSER_ENVIRONMENT ||
    request.environment !== process.env.ARGUS_BROWSER_ENVIRONMENT ||
    request.testRevision?.digest !== (await gitHead())
  )
    fail("invalid request or checkout revision");
  const registry = await requestedRegistry();
  if (request.apiVersion === "argus.dev/browser-adapter-request/v2")
    await verifyStagedInventory(request, registry);
  const startedAt = new Date().toISOString();
  const root = await mkdtemp(join(tmpdir(), "argus-browser-"));
  try {
    const results = [];
    const artifacts = [];
    const seen = new Set();
    for (const test of request.tests) {
      const key = `${test.suiteKey}/${test.testKey}`;
      if (seen.has(key)) fail("duplicate requested test");
      seen.add(key);
      const value = await oneTest(request, test, registry, root);
      results.push(value.normalized);
      artifacts.push(...value.artifacts);
    }
    process.stdout.write(
      `${JSON.stringify({
        apiVersion: "argus.dev/browser-adapter-result/v1",
        attemptId: request.attemptId,
        adapter: { id: "playwright", version },
        project: request.project,
        environment: request.environment,
        startedAt,
        completedAt: new Date().toISOString(),
        results,
        artifacts,
      })}\n`,
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}
main().catch((error) => {
  process.stderr.write(`${error.message}\n`);
  process.exitCode = 1;
});
