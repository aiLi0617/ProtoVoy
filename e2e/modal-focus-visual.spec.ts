import { expect, test } from "@playwright/test";

test("captures the accessible execution-history drawer", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop-chromium", "Desktop visual capture only");
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-focus-history")));
  await expect(page.getByRole("dialog", { name: "执行历史" })).toBeVisible();
  await page.screenshot({ path: "output/playwright/modal-focus-execution-history.png", fullPage: true });
});
