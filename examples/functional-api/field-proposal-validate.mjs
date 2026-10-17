// Exercise the reviewed field adapter against a disposable immutable checkout.
import { spawn, spawnSync } from "node:child_process";
import { cp, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve, sep } from "node:path";

const fixture = resolve("examples/functional-api/repair-test-repository");
const adapter = resolve("examples/functional-api/playwright-repair-adapter.mjs");
const workspace = await mkdtemp(join(tmpdir(), "argus-field-proposal-"));
const sourcePath = join(workspace, "tests", "fields.spec.mjs");

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
    "field-proposal-validate: request, response, stale and dynamic source checks passed\n",
  );
} finally {
  await rm(workspace, { recursive: true, force: true });
}
