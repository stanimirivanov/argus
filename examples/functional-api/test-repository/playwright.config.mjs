import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  reporter: "json",
  workers: 1,
  retries: 0,
  use: { baseURL: process.env.ARGUS_FIXTURE_URL },
});
