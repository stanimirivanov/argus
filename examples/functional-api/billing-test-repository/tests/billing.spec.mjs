import { expect, test } from "@playwright/test";

test("billing health [argus:billing-api:billing-health-test]", async ({ request }) => {
  const response = await request.get("/billing/health");
  expect(response.status()).toBe(200);
});
