import { writeFile } from "node:fs/promises";
import { expect, test } from "@playwright/test";

test.afterEach(async ({ page }, info) => {
  const ariaSnapshot = await page.locator("body").ariaSnapshot();
  const nodes = await page.locator("body *").evaluateAll((elements) =>
    elements
      .filter((element) => element.matches("button,a,input,select,textarea,[role],[data-testid]"))
      .map((element) => {
        const role =
          element.getAttribute("role") ??
          { BUTTON: "button", A: "link", INPUT: "textbox" }[element.tagName] ??
          "";
        const name = element.getAttribute("aria-label") ?? element.textContent?.trim() ?? "";
        const style = getComputedStyle(element);
        return {
          role,
          name,
          testId: element.getAttribute("data-testid") ?? "",
          visible:
            style.display !== "none" &&
            style.visibility !== "hidden" &&
            element.getBoundingClientRect().width > 0 &&
            element.getBoundingClientRect().height > 0,
        };
      }),
  );
  const path = info.outputPath("accessibility.json");
  await writeFile(
    path,
    JSON.stringify({
      apiVersion: "argus.dev/browser-structure/v1",
      capturedAt: new Date().toISOString(),
      url: page.url(),
      complete: nodes.length < 10_000,
      nodes: nodes.slice(0, 10_000),
      ariaSnapshot,
    }),
    "utf8",
  );
  await info.attach("argus-accessibility", { path, contentType: "application/json" });
});

test("checkout shows total [argus:shop:checkout]", async ({ page }) => {
  const actionName = process.env.ARGUS_BROWSER_RENAME_CHECKOUT ? "Place order" : "Checkout";
  await page.setContent(
    `<main><button data-testid="checkout-action">${actionName}</button><p aria-label="Total">€42.00</p></main>`,
  );
  await expect(page.getByRole("button", { name: "Checkout" })).toBeVisible();
  await expect(page.getByLabel("Total")).toHaveText("€42.00");
});

test("receipt shows order [argus:shop:receipt]", async ({ page }) => {
  await page.setContent("<main><h1>Receipt</h1><p>Order 123</p></main>");
  await expect(
    page.getByText(process.env.ARGUS_BROWSER_FAIL_RECEIPT ? "Order 999" : "Order 123"),
  ).toBeVisible();
});
