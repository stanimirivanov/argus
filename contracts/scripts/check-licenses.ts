import { spawnSync } from "node:child_process";

const allowedRuntimeLicenses = new Set([
  "Apache-2.0",
  "BSD-2-Clause",
  "BSD-3-Clause",
  "ISC",
  "MIT",
]);

const result =
  process.platform === "win32"
    ? spawnSync("pnpm licenses list --prod --json", { encoding: "utf8", shell: true })
    : spawnSync("pnpm", ["licenses", "list", "--prod", "--json"], { encoding: "utf8" });

if (result.status !== 0) {
  process.stderr.write(result.stderr);
  throw new Error("pnpm could not inventory production licenses");
}

const inventory: unknown = JSON.parse(result.stdout);
if (typeof inventory !== "object" || inventory === null || Array.isArray(inventory)) {
  throw new Error("pnpm returned an invalid production license inventory");
}

const rejected = Object.keys(inventory).filter((license) => !allowedRuntimeLicenses.has(license));
if (rejected.length > 0) {
  throw new Error(`production dependencies use unapproved licenses: ${rejected.join(", ")}`);
}

process.stdout.write(
  `Production dependency licenses satisfy policy: ${Object.keys(inventory).sort().join(", ")}\n`,
);
