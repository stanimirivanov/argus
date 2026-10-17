// Exercise the reviewed field adapter against a disposable immutable checkout.
import { spawn, spawnSync } from "node:child_process";
import { cp, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { join, resolve, sep } from "node:path";

const fixture = resolve("examples/functional-api/repair-test-repository");
const adapter = resolve("examples/functional-api/playwright-repair-adapter.mjs");
const workspace = await mkdtemp(join(tmpdir(), "argus-field-proposal-"));
const sourcePath = join(workspace, "tests", "fields.spec.mjs");
let server;

function command(executable, args, input) {
  const result = spawnSync(executable, args, {
    cwd: workspace,
    input,
    encoding: "utf8",
    windowsHide: true,
    maxBuffer: 8 << 20,
    timeout: 120000,
  });
  if (result.error || result.status !== 0)
    throw new Error(`${executable} failed: ${result.error?.message ?? result.stderr}`);
  return result.stdout.trim();
}

async function install(args) {
  const pnpmScript = process.env.npm_execpath;
  const child = spawn(
    pnpmScript ? process.execPath : "pnpm",
    pnpmScript ? [pnpmScript, ...args] : args,
    {
      cwd: workspace,
      windowsHide: true,
      stdio: "ignore",
      shell: !pnpmScript && process.platform === "win32",
    },
  );
  const code = await new Promise((done, reject) => {
    child.once("error", reject);
    child.once("close", done);
  });
  if (code !== 0) throw new Error(`pnpm install exited ${code}`);
}

function revision() {
  return command("git", ["rev-parse", "HEAD"]);
}

function request(direction, testKey, previousName, name) {
  return {
    apiVersion: "argus.dev/functional-api-field-adaptation-request/v1",
    proposalId: "a".repeat(64),
    test: {
      adapter: "playwright",
      revision: { algorithm: "git-sha1", digest: revision() },
      suiteKey: "orders-api",
      testKey,
    },
    fieldRename: {
      method: direction === "request" ? "POST" : "GET",
      path: "/orders",
      direction,
      previousName,
      name,
    },
  };
}

function proposal(input) {
  return JSON.parse(command(process.execPath, [adapter, "propose-field"], JSON.stringify(input)));
}

function fullProposal(input, result) {
  const sourceRepository = {
    provider: "github",
    host: "github.com",
    providerRepositoryId: "fixture-source-field",
    owner: "argus-fixture",
    name: "field-source",
  };
  const testRepository = {
    provider: "github",
    host: "github.com",
    providerRepositoryId: "fixture-tests-field",
    owner: "argus-fixture",
    name: "field-tests",
  };
  const isRequest = input.fieldRename.direction === "request";
  return {
    apiVersion: "argus.dev/field-adaptation-proposal/v1",
    policyVersion: "argus.dev/adaptation-policy/functional-api-compatible-field-rename/v1",
    proposalId: input.proposalId,
    sourceImpact: {
      apiVersion: "argus.dev/capability-impact/v1",
      analyzerVersion: "argus-openapi/v1+libopenapi/v0.38.7",
    },
    change: {
      sourceRepository,
      pullRequest: { number: 42 },
      baseRevision: { algorithm: "git-sha1", digest: "1".repeat(40) },
      headRevision: { algorithm: "git-sha1", digest: "2".repeat(40) },
      observedAt: "2026-10-17T10:00:00Z",
      trigger: {
        provider: "github",
        deliveryId: "field-fixture",
        event: "pull_request",
        action: "synchronize",
      },
    },
    test: {
      repository: testRepository,
      revision: input.test.revision,
      suiteKey: "orders-api",
      testKey: input.test.testKey,
      name: `${isRequest ? "request" : "response"} field [argus:orders-api:${input.test.testKey}]`,
      adapter: "playwright",
      capabilities: ["orders"],
    },
    classification: "INVALIDATED",
    decision: "PATCH_AND_VALIDATE",
    fieldRename: {
      documentPath: "api/orders.yaml",
      baseSha256: "1".repeat(64),
      headSha256: "2".repeat(64),
      method: isRequest ? "POST" : "GET",
      path: "/orders",
      operationId: "orders",
      capabilities: ["orders"],
      direction: input.fieldRename.direction,
      statusCode: isRequest ? "" : "200",
      previousName: input.fieldRename.previousName,
      name: input.fieldRename.name,
    },
    adapter: result.adapter,
    edit: result.edit,
  };
}

async function validate(executable, proposalPath, serviceURL) {
  const child = spawn(
    executable,
    [
      "-proposal",
      proposalPath,
      "-disposable-workspace",
      workspace,
      "--",
      process.execPath,
      adapter,
      "validate-field",
    ],
    {
      cwd: workspace,
      windowsHide: true,
      env: { ...process.env, ARGUS_FIXTURE_URL: serviceURL },
    },
  );
  const stdout = [];
  const stderr = [];
  child.stdout.on("data", (chunk) => stdout.push(chunk));
  child.stderr.on("data", (chunk) => stderr.push(chunk));
  const status = await new Promise((done, reject) => {
    child.once("error", reject);
    child.once("close", done);
  });
  return {
    status,
    stdout: Buffer.concat(stdout).toString("utf8"),
    stderr: Buffer.concat(stderr).toString("utf8"),
  };
}

function assertEdit(result, source, previousName, name, role) {
  if (
    result.outcome !== "candidate" ||
    result.edit?.semanticRole !== role ||
    result.edit.original !== previousName ||
    result.edit.replacement !== name
  )
    throw new Error(`unexpected field edit: ${JSON.stringify(result)}`);
  const bytes = Buffer.from(source);
  if (bytes.subarray(result.edit.startByte, result.edit.endByte).toString("utf8") !== previousName)
    throw new Error("field edit byte span misses the exact preimage");
  const updated = Buffer.concat([
    bytes.subarray(0, result.edit.startByte),
    Buffer.from(name),
    bytes.subarray(result.edit.endByte),
  ]).toString("utf8");
  if (updated !== source.replace(previousName, name))
    throw new Error("field proposal changed more than the selected identifier");
}

try {
  await cp(fixture, workspace, {
    recursive: true,
    filter: (path) =>
      !["node_modules", "test-results", "playwright-report", ".pnpm-store"].some(
        (name) => path.includes(`${sep}${name}${sep}`) || path.endsWith(`${sep}${name}`),
      ),
  });
  try {
    await install(["install", "--frozen-lockfile", "--offline"]);
  } catch {
    await install(["install", "--frozen-lockfile"]);
  }
  await mkdir(join(workspace, "tests"), { recursive: true });
  const source = `import { expect, test } from "@playwright/test";
test("request field [argus:orders-api:request-field]", async ({ request }) => {
  const response = await request.post("/orders", { data: { customerId: "buyer-1" } });
  expect(response.status()).toBe(200);
});
test("response field [argus:orders-api:response-field]", async ({ request }) => {
  const response = await request.get("/orders");
  expect((await response.json()).orderId).toBe("order-1");
});
`;
  await writeFile(sourcePath, source);
  await writeFile(
    join(workspace, "argus-tests.json"),
    JSON.stringify({
      apiVersion: "argus.dev/reference-playwright-test-map/v1",
      tests: ["request-field", "response-field"].map((testKey) => ({
        suiteKey: "orders-api",
        testKey,
        tag: `argus:orders-api:${testKey}`,
        file: "tests/fields.spec.mjs",
      })),
    }),
  );
  command("git", ["init", "-q"]);
  command("git", ["config", "user.email", "argus-fixture@example.invalid"]);
  command("git", ["config", "user.name", "Argus fixture"]);
  command("git", ["add", "."]);
  command("git", ["commit", "-qm", "Create field adapter fixture"]);

  assertEdit(
    proposal(request("request", "request-field", "customerId", "clientId")),
    source,
    "customerId",
    "clientId",
    "request-body-key",
  );
  assertEdit(
    proposal(request("response", "response-field", "orderId", "purchaseId")),
    source,
    "orderId",
    "purchaseId",
    "response-field-access",
  );
  if ((await readFile(sourcePath, "utf8")) !== source) throw new Error("proposal mutated source");

  const validationCommand = resolve(
    workspace,
    process.platform === "win32" ? "field-validation.exe" : "field-validation",
  );
  const built = spawnSync(
    "go",
    ["build", "-o", validationCommand, "./cmd/validate-functional-api-field-repair"],
    {
      cwd: resolve("."),
      encoding: "utf8",
      windowsHide: true,
      timeout: 120000,
    },
  );
  if (built.status !== 0) throw new Error(`field validation build failed: ${built.stderr}`);
  let falseHealing = false;
  server = createServer(async (req, res) => {
    let body = "";
    for await (const chunk of req) body += chunk;
    let status = 404;
    let value = { error: "not-found" };
    if (req.method === "POST" && req.url === "/orders") {
      const data = JSON.parse(body);
      status = data.clientId === "buyer-1" ? 200 : 400;
      value = { accepted: status === 200 };
    } else if (req.method === "GET" && req.url === "/orders") {
      status = 200;
      value = {
        purchaseId: "order-1",
        ...(falseHealing ? { [`__argus_negative_field_${"a".repeat(12)}`]: "order-1" } : {}),
      };
    }
    res.writeHead(status, { "content-type": "application/json" });
    res.end(JSON.stringify(value));
  });
  await new Promise((done) => server.listen(0, "127.0.0.1", done));
  const serviceURL = `http://127.0.0.1:${server.address().port}`;
  for (const [direction, testKey, previousName, name] of [
    ["request", "request-field", "customerId", "clientId"],
    ["response", "response-field", "orderId", "purchaseId"],
  ]) {
    const input = request(direction, testKey, previousName, name);
    const result = proposal(input);
    const proposalPath = join(workspace, `${direction}-proposal.json`);
    await writeFile(proposalPath, JSON.stringify(fullProposal(input, result)));
    const before = await readFile(sourcePath);
    const checked = await validate(validationCommand, proposalPath, serviceURL);
    if (checked.status !== 0)
      throw new Error(`${direction} field validation failed: ${checked.stderr}`);
    const evidence = JSON.parse(checked.stdout);
    if (
      evidence.apiVersion !== "argus.dev/field-validation-evidence/v1" ||
      evidence.runs.map((run) => run.outcome).join(",") !==
        "failed,failed,passed,passed,failed,failed" ||
      !before.equals(await readFile(sourcePath))
    )
      throw new Error(`${direction} field proof or restoration failed`);
    if (direction === "response") {
      falseHealing = true;
      const rejected = await validate(validationCommand, proposalPath, serviceURL);
      if (
        rejected.status === 0 ||
        JSON.parse(rejected.stdout).rejection?.reason !== "negative-control-passed" ||
        !before.equals(await readFile(sourcePath))
      )
        throw new Error("false-healing response control was accepted");
      falseHealing = false;
    }
  }

  await writeFile(sourcePath, source.replace('data: { customerId: "buyer-1" }', "data: payload"));
  if (
    proposal(request("request", "request-field", "customerId", "clientId")).reasonCode !==
    "stale-source"
  )
    throw new Error("uncommitted source was accepted");
  command("git", ["add", "."]);
  command("git", ["commit", "-qm", "Use dynamic request body"]);
  if (
    proposal(request("request", "request-field", "customerId", "clientId")).outcome !== "abstained"
  )
    throw new Error("dynamic request body was accepted");
  process.stdout.write(
    "field-proposal-validate: request/response repairs, repeated controls, false healing and source checks passed\n",
  );
} finally {
  if (server) await new Promise((done) => server.close(done));
  await rm(workspace, { recursive: true, force: true, maxRetries: 20, retryDelay: 250 });
}
