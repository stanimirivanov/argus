// Reference protocol adapter. Run it from a reviewed test-repository checkout.
import { spawn } from "node:child_process";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { resolve } from "node:path";

const cwd = process.cwd();
const require = createRequire(resolve(cwd, "package.json"));
const playwrightCLI = require.resolve("@playwright/test/cli");
const version = require("@playwright/test/package.json").version;
const maxOutput = 8 << 20;
const keyPattern = /^[a-z0-9][a-z0-9._-]*$/u;
const revisionPattern = /^[0-9a-f]{40}$/u;
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

async function run(command, args, options = {}) {
  const child = spawn(command, args, { cwd, windowsHide: true, ...options });
  activeChild = child;
  const output = [];
  const errors = [];
  let outputSize = 0;
  let errorSize = 0;
  child.stdout.on("data", (chunk) => {
    outputSize += chunk.length;
    if (outputSize > maxOutput) child.kill();
    else output.push(chunk);
  });
  child.stderr.on("data", (chunk) => {
    errorSize += chunk.length;
    if (errorSize <= 64 << 10) errors.push(chunk);
  });
  const code = await new Promise((done, reject) => {
    child.once("error", reject);
    child.once("close", done);
  });
  activeChild = undefined;
  if (outputSize > maxOutput) fail("Playwright output exceeded 8 MiB");
  return {
    code,
    stdout: Buffer.concat(output).toString("utf8"),
    stderr: Buffer.concat(errors).toString("utf8"),
  };
}

async function git(...args) {
  const result = await run("git", args);
  if (result.code !== 0) fail(`git ${args[0]} failed: ${result.stderr.trim()}`);
  return result.stdout.trim();
}

async function currentRevision() {
  const revision = await git("rev-parse", "HEAD");
  if (!revisionPattern.test(revision))
    fail("test checkout must have an immutable git-sha1 revision");
  return revision;
}

function parseJSON(data, label) {
  try {
    return JSON.parse(data);
  } catch {
    fail(`malformed ${label} JSON`);
  }
}

async function registry() {
  const document = parseJSON(await readFile(resolve(cwd, "argus-tests.json"), "utf8"), "test map");
  if (
    document.apiVersion !== "argus.dev/reference-playwright-test-map/v1" ||
    !Array.isArray(document.tests)
  ) {
    fail("unsupported test map");
  }
  const byKey = new Map();
  const byTag = new Map();
  for (const item of document.tests) {
    const key = `${item.suiteKey}/${item.testKey}`;
    if (
      !keyPattern.test(item.suiteKey) ||
      !keyPattern.test(item.testKey) ||
      item.tag !== `argus:${item.suiteKey}:${item.testKey}` ||
      typeof item.file !== "string" ||
      !item.file.startsWith("tests/") ||
      byKey.has(key) ||
      byTag.has(item.tag)
    )
      fail("ambiguous test map");
    byKey.set(key, item);
    byTag.set(item.tag, item);
  }
  return { byKey, byTag };
}

function flatten(suites) {
  const specs = [];
  for (const suite of suites ?? []) {
    for (const spec of suite.specs ?? []) specs.push(spec);
    specs.push(...flatten(suite.suites ?? []));
  }
  return specs;
}

async function discover() {
  const map = await registry();
  const result = await run(process.execPath, [playwrightCLI, "test", "--list", "--reporter=json"]);
  if (result.code !== 0) fail(`Playwright discovery failed: ${result.stderr.trim()}`);
  const report = parseJSON(result.stdout, "Playwright discovery");
  if (
    !Array.isArray(report.suites) ||
    report.errors?.length ||
    report.config?.version !== version
  ) {
    fail("incomplete Playwright discovery");
  }
  const discovered = new Map();
  for (const spec of flatten(report.suites)) {
    const tags = [...spec.title.matchAll(/\[(argus:[a-z0-9._:-]+)\]/gu)];
    if (tags.length !== 1 || !Array.isArray(spec.tests) || spec.tests.length !== 1) {
      fail("Playwright test lacks one stable Argus tag or has multiple variants");
    }
    const item = map.byTag.get(tags[0][1]);
    const file = `tests/${spec.file}`.replaceAll("\\", "/");
    if (!item || item.file !== file || discovered.has(`${item.suiteKey}/${item.testKey}`)) {
      fail("Playwright inventory differs from reviewed test map");
    }
    discovered.set(`${item.suiteKey}/${item.testKey}`, { ...item, name: spec.title });
  }
  if (discovered.size !== map.byKey.size) fail("reviewed test map contains undiscovered tests");
  return discovered;
}

async function inventory() {
  const sourceRepository = parseJSON(
    process.env.ARGUS_SOURCE_REPOSITORY ?? "",
    "source repository",
  );
  const testRepository = parseJSON(process.env.ARGUS_TEST_REPOSITORY ?? "", "test repository");
  const revision = await currentRevision();
  const baseRevision = process.env.ARGUS_BASE_TEST_REVISION ?? revision;
  if (!revisionPattern.test(baseRevision)) fail("invalid base test revision");
  const baseMap = parseJSON(await git("show", `${baseRevision}:argus-tests.json`), "base test map");
  if (
    baseMap.apiVersion !== "argus.dev/reference-playwright-test-map/v1" ||
    !Array.isArray(baseMap.tests)
  ) {
    fail("invalid base test map");
  }
  const baseline = new Map(baseMap.tests.map((item) => [`${item.suiteKey}/${item.testKey}`, item]));
  if (baseline.size !== baseMap.tests.length) fail("duplicate base test identity");
  const changed = new Set(
    (await git("diff", "--name-only", baseRevision, revision, "--", "tests", "argus-tests.json"))
      .split(/\r?\n/u)
      .filter(Boolean)
      .map((path) => path.replaceAll("\\", "/")),
  );
  const observed = await discover();
  const tests = [...observed.values()].map((item) => {
    const previous = baseline.get(`${item.suiteKey}/${item.testKey}`);
    return {
      testRepository,
      revision: { algorithm: "git-sha1", digest: revision },
      suiteKey: item.suiteKey,
      adapter: "playwright",
      testKey: item.testKey,
      name: item.name,
      capabilities: [],
      new: !previous,
      modified:
        !!previous &&
        (changed.has(item.file) ||
          previous.file !== item.file ||
          previous.tag !== item.tag ||
          changed.has("argus-tests.json")),
    };
  });
  tests.sort((left, right) =>
    `${left.suiteKey}/${left.testKey}`.localeCompare(`${right.suiteKey}/${right.testKey}`),
  );
  process.stdout.write(
    `${JSON.stringify({
      apiVersion: "argus.dev/head-test-inventory/v1",
      sourceRepository,
      complete: true,
      tests,
    })}\n`,
  );
}

async function execute() {
  let input = "";
  for await (const chunk of process.stdin) {
    input += chunk;
    if (input.length > 16 << 20) fail("adapter request exceeds 16 MiB");
  }
  const request = parseJSON(input, "adapter request");
  if (
    request.apiVersion !== "argus.dev/functional-api-adapter-request/v1" ||
    request.adapter !== "playwright" ||
    !Array.isArray(request.tests) ||
    request.tests.length === 0 ||
    request.testRevision?.digest !== (await currentRevision()) ||
    request.testRevision?.algorithm !== "git-sha1"
  )
    fail("invalid adapter request or checkout revision");
  const discovered = await discover();
  const startedAt = new Date().toISOString();
  const results = [];
  const requested = new Set();
  for (const test of request.tests) {
    const key = `${test.suiteKey}/${test.testKey}`;
    const item = discovered.get(key);
    if (!item || item.name !== test.name || requested.has(key))
      fail("requested test differs from discovered inventory");
    requested.add(key);
    const grep = `\\[${item.tag.replaceAll(":", "\\:")}\\]`;
    const runResult = await run(process.execPath, [
      playwrightCLI,
      "test",
      "--reporter=json",
      "--grep",
      grep,
    ]);
    const report = parseJSON(runResult.stdout, "Playwright result");
    const specs = flatten(report.suites);
    if (
      report.errors?.length ||
      specs.length !== 1 ||
      specs[0].title !== item.name ||
      specs[0].tests?.length !== 1 ||
      specs[0].tests[0].results?.length !== 1
    ) {
      fail("Playwright result does not match the requested test");
    }
    const attempt = specs[0].tests[0].results[0];
    const outcome =
      attempt.status === "passed"
        ? "passed"
        : attempt.status === "failed"
          ? "failed"
          : attempt.status === "skipped"
            ? "skipped"
            : "error";
    if ((runResult.code === 0) !== (outcome === "passed" || outcome === "skipped")) {
      fail("Playwright exit status contradicts its result");
    }
    results.push({
      suiteKey: test.suiteKey,
      testKey: test.testKey,
      outcome,
      durationMs: Math.max(0, Math.min(86_400_000, Math.trunc(attempt.duration ?? 0))),
      failure:
        outcome === "passed" || outcome === "skipped"
          ? null
          : {
              code: outcome === "failed" ? "assertion-failed" : "playwright-error",
              message:
                outcome === "failed"
                  ? "Playwright assertion failed"
                  : "Playwright test did not complete",
            },
    });
  }
  process.stdout.write(
    `${JSON.stringify({
      apiVersion: "argus.dev/functional-api-adapter-result/v1",
      attemptId: request.attemptId,
      adapter: { id: "playwright", version },
      startedAt,
      completedAt: new Date().toISOString(),
      results,
      artifacts: [],
    })}\n`,
  );
}

try {
  if (process.argv[2] === "discover") await inventory();
  else if (process.argv[2] === "execute") await execute();
  else fail("usage: playwright-adapter.mjs discover|execute");
} catch (error) {
  process.stderr.write(`${error.message}\n`);
  process.exitCode = 1;
}
