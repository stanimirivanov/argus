import { expect, test } from "@playwright/test";

test("create order [argus:orders-api:create-order-test]", async ({ request }) => {
  const response = await request.post("/orders", { data: { sku: "book" } });
  expect(response.status()).toBe(201);
  expect((await response.json()).id).toBe("order-1");
});

test("list orders [argus:orders-api:list-orders-test]", async ({ request }) => {
  const response = await request.get("/orders");
  expect(response.status()).toBe(200);
  expect((await response.json()).items).toHaveLength(1);
});
