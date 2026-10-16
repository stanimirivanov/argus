import { expect, test } from "@playwright/test";

test("list renamed orders [argus:orders-api:renamed-orders-test]", async ({ request }) => {
  const response = await request.get("/v1/orders");
  expect(response.status()).toBe(200);
  expect((await response.json()).items).toHaveLength(1);
});
