import { writeFile } from "node:fs/promises";
import { expect, test } from "@playwright/test";

test.afterEach(async ({ page }, info) => {
  const snapshot = await page.locator("body").ariaSnapshot();
  const path = info.outputPath("accessibility.txt");
  await writeFile(path, snapshot, "utf8");
  await info.attach("argus-accessibility", { path, contentType: "text/plain" });
});

test("checkout shows total [argus:shop:checkout]", async ({ page }) => {
  await page.setContent('<main><h1>Checkout</h1><p aria-label="Total">€42.00</p></main>');
  await expect(page.getByLabel("Total")).toHaveText("€42.00");
});

test("receipt shows order [argus:shop:receipt]", async ({ page }) => {
  await page.setContent("<main><h1>Receipt</h1><p>Order 123</p></main>");
  await expect(page.getByText("Order 123")).toBeVisible();
});
