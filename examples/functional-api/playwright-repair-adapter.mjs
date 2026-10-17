// Reviewed reference adapter for one literal Playwright request target.
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { resolve } from "node:path";

const cwd = process.cwd();
const require = createRequire(resolve(cwd, "package.json"));
const ts = require("typescript");
const playwrightCLI = require.resolve("@playwright/test/cli");
const version = require("@playwright/test/package.json").version;
const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");

function fail(message) {
  throw new Error(message);
}

async function input() {
  let value = "";
  for await (const chunk of process.stdin) {
    value += chunk;
    if (value.length > 16 << 20) fail("adapter request exceeds 16 MiB");
  }
  try {
    return JSON.parse(value);
  } catch {
    fail("malformed adapter request");
  }
}

async function revision() {
  const result = await run("git", ["rev-parse", "HEAD"]);
  const value = result.stdout.trim();
  if (result.code !== 0 || !/^[0-9a-f]{40}$/u.test(value)) fail("invalid test checkout revision");
  return value;
}

async function run(executable, args) {
  const child = spawn(executable, args, { cwd, windowsHide: true });
  const stdout = [];
  const stderr = [];
  let size = 0;
  child.stdout.on("data", (chunk) => {
    size += chunk.length;
    if (size > 8 << 20) child.kill();
    else stdout.push(chunk);
  });
  child.stderr.on("data", (chunk) => {
    if (Buffer.concat(stderr).length < 64 << 10) stderr.push(chunk);
  });
  const code = await new Promise((done, reject) => {
    child.once("error", reject);
    child.once("close", done);
  });
  if (size > 8 << 20) fail("process output exceeds 8 MiB");
  return {
    code,
    stdout: Buffer.concat(stdout).toString("utf8"),
    stderr: Buffer.concat(stderr).toString("utf8"),
  };
}

async function mappedTest(test) {
  const map = JSON.parse(await readFile(resolve(cwd, "argus-tests.json"), "utf8"));
  if (map.apiVersion !== "argus.dev/reference-playwright-test-map/v1" || !Array.isArray(map.tests))
    fail("invalid reviewed test map");
  const matches = map.tests.filter(
    (item) => item.suiteKey === test.suiteKey && item.testKey === test.testKey,
  );
  if (
    matches.length !== 1 ||
    matches[0].tag !== `argus:${test.suiteKey}:${test.testKey}` ||
    !/^tests\/[a-zA-Z0-9_/-]+\.spec\.mjs$/u.test(matches[0].file)
  )
    fail("unmapped test");
  return matches[0];
}

function abstained(request, code, reason) {
  return {
    apiVersion: "argus.dev/functional-api-adaptation-result/v1",
    proposalId: request.proposalId,
    adapter: { id: "playwright", version },
    outcome: "abstained",
    reasonCode: code,
    reason,
    edit: null,
  };
}

function findTarget(source, tag, previousPath, method) {
  const tree = ts.createSourceFile(
    "orders.spec.mjs",
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.JS,
  );
  if (tree.parseDiagnostics.length) return null;
  const matches = [];
  let taggedTests = 0;
  let requestCalls = 0;
  function visit(node, inTest) {
    let selected = inTest;
    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === "test"
    ) {
      const tagged =
        node.arguments.length === 2 &&
        ts.isStringLiteral(node.arguments[0]) &&
        node.arguments[0].text.includes(`[${tag}]`);
      if (tagged) {
        taggedTests += 1;
        const callback = node.arguments[1];
        selected =
          ts.isArrowFunction(callback) &&
          callback.parameters.length === 1 &&
          ts.isObjectBindingPattern(callback.parameters[0].name) &&
          callback.parameters[0].name.elements.some(
            (element) =>
              ts.isIdentifier(element.name) &&
              element.name.text === "request" &&
              !element.propertyName,
          );
      } else selected = false;
    }
    if (
      selected &&
      ts.isCallExpression(node) &&
      ts.isPropertyAccessExpression(node.expression) &&
      ts.isIdentifier(node.expression.expression) &&
      node.expression.expression.text === "request" &&
      node.expression.name.text.toUpperCase() === method
    ) {
      requestCalls += 1;
      if (
        node.arguments.length === 1 &&
        ts.isStringLiteral(node.arguments[0]) &&
        node.arguments[0].text === previousPath
      )
        matches.push(node.arguments[0]);
    }
    ts.forEachChild(node, (child) => visit(child, selected));
  }
  visit(tree, false);
  if (taggedTests !== 1 || requestCalls !== 1 || matches.length !== 1) return null;
  const literal = matches[0];
  const raw = source.slice(literal.getStart(tree), literal.getEnd());
  if (raw !== `"${previousPath}"` && raw !== `'${previousPath}'`) return null;
  return [literal.getStart(tree) + 1, literal.getEnd() - 1];
}

async function propose(request) {
  if (
    request.apiVersion !== "argus.dev/functional-api-adaptation-request/v1" ||
    request.test?.adapter !== "playwright" ||
    request.test?.revision?.digest !== (await revision()) ||
    request.endpointRename?.method !== "GET"
  )
    fail("invalid proposal request");
  const mapped = await mappedTest(request.test);
  const bytes = await readFile(resolve(cwd, mapped.file));
  const source = bytes.toString("utf8");
  if (!Buffer.from(source).equals(bytes))
    return abstained(request, "unsupported-source", "source is not UTF-8");
  const span = findTarget(
    source,
    mapped.tag,
    request.endpointRename.previousPath,
    request.endpointRename.method,
  );
  if (!span)
    return abstained(request, "ambiguous-target", "one literal request target could not be proven");
  const [start, end] = span.map((offset) => Buffer.byteLength(source.slice(0, offset)));
  return {
    apiVersion: "argus.dev/functional-api-adaptation-result/v1",
    proposalId: request.proposalId,
    adapter: { id: "playwright", version },
    outcome: "candidate",
    reasonCode: null,
    reason: null,
    edit: {
      path: mapped.file,
      beforeSha256: digest(bytes),
      startByte: start,
      endByte: end,
      original: request.endpointRename.previousPath,
      replacement: request.endpointRename.path,
      semanticRole: "request-target",
    },
  };
}

function fieldAbstention(request, code, reason) {
  return {
    apiVersion: "argus.dev/functional-api-field-adaptation-result/v1",
    proposalId: request.proposalId,
    adapter: { id: "playwright", version },
    outcome: "abstained",
    reasonCode: code,
    reason,
    edit: null,
  };
}

function taggedCallback(node, tag) {
  return (
    ts.isCallExpression(node) &&
    ts.isIdentifier(node.expression) &&
    node.expression.text === "test" &&
    node.arguments.length === 2 &&
    ts.isStringLiteral(node.arguments[0]) &&
    node.arguments[0].text.includes(`[${tag}]`) &&
    ts.isArrowFunction(node.arguments[1]) &&
    node.arguments[1].parameters.length === 1 &&
    ts.isObjectBindingPattern(node.arguments[1].parameters[0].name) &&
    node.arguments[1].parameters[0].name.elements.some(
      (element) =>
        ts.isIdentifier(element.name) && element.name.text === "request" && !element.propertyName,
    )
  );
}

function unwrapParentheses(node) {
  while (ts.isParenthesizedExpression(node)) node = node.expression;
  return node;
}

function requestCall(node, rename) {
  if (!ts.isCallExpression(node) || !ts.isPropertyAccessExpression(node.expression)) return false;
  const receiver = node.expression.expression;
  return (
    ts.isIdentifier(receiver) &&
    receiver.text === "request" &&
    node.expression.name.text.toUpperCase() === rename.method &&
    node.arguments.length >= 1 &&
    ts.isStringLiteral(node.arguments[0]) &&
    node.arguments[0].text === rename.path
  );
}

function requestFieldNode(call, previousName) {
  if (call.arguments.length !== 2 || !ts.isObjectLiteralExpression(call.arguments[1])) return null;
  const data = call.arguments[1].properties.filter(
    (property) =>
      ts.isPropertyAssignment(property) &&
      ts.isIdentifier(property.name) &&
      property.name.text === "data",
  );
  if (data.length !== 1 || !ts.isObjectLiteralExpression(data[0].initializer)) return null;
  const properties = data[0].initializer.properties;
  const matches = properties.filter(
    (property) =>
      ts.isPropertyAssignment(property) &&
      ts.isIdentifier(property.name) &&
      property.name.text === previousName,
  );
  return matches.length === 1 ? matches[0].name : null;
}

function responseVariable(node, rename) {
  if (!ts.isVariableDeclaration(node) || !ts.isIdentifier(node.name) || !node.initializer)
    return null;
  const initializer = unwrapParentheses(node.initializer);
  return ts.isAwaitExpression(initializer) && requestCall(initializer.expression, rename)
    ? node.name.text
    : null;
}

function responseFieldNode(node, variable, previousName) {
  if (
    !ts.isCallExpression(node) ||
    !ts.isIdentifier(node.expression) ||
    node.expression.text !== "expect" ||
    node.arguments.length !== 1
  )
    return null;
  const access = unwrapParentheses(node.arguments[0]);
  if (!ts.isPropertyAccessExpression(access) || access.name.text !== previousName) return null;
  const awaited = unwrapParentheses(access.expression);
  if (!ts.isAwaitExpression(awaited)) return null;
  const json = awaited.expression;
  if (
    !ts.isCallExpression(json) ||
    json.arguments.length !== 0 ||
    !ts.isPropertyAccessExpression(json.expression) ||
    json.expression.name.text !== "json" ||
    !ts.isIdentifier(json.expression.expression) ||
    json.expression.expression.text !== variable
  )
    return null;
  return access.name;
}

function findField(source, tag, rename) {
  const tree = ts.createSourceFile(
    "field.spec.mjs",
    source,
    ts.ScriptTarget.Latest,
    true,
    ts.ScriptKind.JS,
  );
  if (tree.parseDiagnostics.length) return null;
  const tagged = [];
  function collect(node) {
    if (taggedCallback(node, tag)) tagged.push(node.arguments[1]);
    ts.forEachChild(node, collect);
  }
  collect(tree);
  if (tagged.length !== 1) return null;
  const calls = [];
  const variables = [];
  function inspect(node) {
    if (requestCall(node, rename)) calls.push(node);
    const variable = responseVariable(node, rename);
    if (variable) variables.push(variable);
    ts.forEachChild(node, inspect);
  }
  inspect(tagged[0]);
  if (calls.length !== 1) return null;
  const matches = [];
  if (rename.direction === "request") {
    const node = requestFieldNode(calls[0], rename.previousName);
    if (node) matches.push(node);
  } else if (rename.direction === "response" && variables.length === 1) {
    function inspectExpect(node) {
      const field = responseFieldNode(node, variables[0], rename.previousName);
      if (field) matches.push(field);
      ts.forEachChild(node, inspectExpect);
    }
    inspectExpect(tagged[0]);
  }
  if (
    matches.length !== 1 ||
    source.slice(matches[0].getStart(tree), matches[0].getEnd()) !== rename.previousName
  )
    return null;
  return [matches[0].getStart(tree), matches[0].getEnd()];
}

async function proposeField(request) {
  if (
    request.apiVersion !== "argus.dev/functional-api-field-adaptation-request/v1" ||
    request.test?.adapter !== "playwright" ||
    request.test?.revision?.digest !== (await revision()) ||
    !["request", "response"].includes(request.fieldRename?.direction) ||
    !/^[A-Za-z_$][A-Za-z0-9_$]{0,254}$/u.test(request.fieldRename.previousName) ||
    !/^[A-Za-z_$][A-Za-z0-9_$]{0,254}$/u.test(request.fieldRename.name)
  )
    fail("invalid field proposal request");
  const mapped = await mappedTest(request.test);
  const bytes = await readFile(resolve(cwd, mapped.file));
  const source = bytes.toString("utf8");
  if (!Buffer.from(source).equals(bytes))
    return fieldAbstention(request, "unsupported-source", "source is not UTF-8");
  const committed = await run("git", ["show", `HEAD:${mapped.file}`]);
  if (committed.code !== 0 || committed.stdout !== source)
    return fieldAbstention(
      request,
      "stale-source",
      "test source differs from its immutable revision",
    );
  const span = findField(source, mapped.tag, request.fieldRename);
  if (!span)
    return fieldAbstention(
      request,
      "ambiguous-field",
      "one supported field reference could not be proven",
    );
  const [startByte, endByte] = span.map((offset) => Buffer.byteLength(source.slice(0, offset)));
  return {
    apiVersion: "argus.dev/functional-api-field-adaptation-result/v1",
    proposalId: request.proposalId,
    adapter: { id: "playwright", version },
    outcome: "candidate",
    reasonCode: null,
    reason: null,
    edit: {
      path: mapped.file,
      beforeSha256: digest(bytes),
      startByte,
      endByte,
      original: request.fieldRename.previousName,
      replacement: request.fieldRename.name,
      semanticRole:
        request.fieldRename.direction === "request" ? "request-body-key" : "response-field-access",
    },
  };
}

function flatten(suites) {
  return (suites ?? []).flatMap((suite) => [...(suite.specs ?? []), ...flatten(suite.suites)]);
}

async function validate(request) {
  if (
    request.apiVersion !== "argus.dev/functional-api-repair-validation-request/v1" ||
    request.test?.adapter !== "playwright" ||
    request.test?.revision?.digest !== (await revision())
  )
    fail("invalid validation request");
  const mapped = await mappedTest(request.test);
  const bytes = await readFile(resolve(cwd, mapped.file));
  if (digest(bytes) !== request.sourceSha256) fail("validation source digest differs");
  const startedAt = new Date().toISOString();
  const result = await run(process.execPath, [
    playwrightCLI,
    "test",
    "--reporter=json",
    "--grep",
    `\\[${mapped.tag}\\]`,
  ]);
  let report;
  try {
    report = JSON.parse(result.stdout);
  } catch {
    fail(`Playwright did not produce JSON: ${result.stderr.slice(-1000)}`);
  }
  const specs = flatten(report.suites);
  if (
    report.errors?.length ||
    specs.length !== 1 ||
    specs[0].title !== request.test.name ||
    specs[0].tests?.length !== 1 ||
    specs[0].tests[0].results?.length !== 1
  )
    fail("Playwright result differs from the requested test");
  const status = specs[0].tests[0].results[0].status;
  const outcome = status === "passed" ? "passed" : status === "failed" ? "failed" : "error";
  if ((result.code === 0) !== (outcome === "passed"))
    fail("Playwright exit status contradicts result");
  return {
    apiVersion: "argus.dev/functional-api-repair-validation-result/v1",
    validationId: request.validationId,
    proposalId: request.proposalId,
    phase: request.phase,
    suiteKey: request.test.suiteKey,
    testKey: request.test.testKey,
    adapter: { id: "playwright", version },
    sourceSha256: request.sourceSha256,
    startedAt,
    completedAt: new Date().toISOString(),
    outcome,
    failure:
      outcome === "passed"
        ? null
        : {
            code: outcome === "failed" ? "assertion-failed" : "playwright-error",
            message:
              outcome === "failed"
                ? "Playwright assertion failed"
                : "Playwright test did not complete",
          },
  };
}

try {
  const request = await input();
  const result =
    process.argv[2] === "propose"
      ? await propose(request)
      : process.argv[2] === "propose-field"
        ? await proposeField(request)
        : process.argv[2] === "validate"
          ? await validate(request)
          : fail("usage: playwright-repair-adapter.mjs propose|validate");
  process.stdout.write(`${JSON.stringify(result)}\n`);
} catch (error) {
  process.stderr.write(`${error.message}\n`);
  process.exitCode = 1;
}
