import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");
});

test("selects a request Content-Type beside the body format and syncs Headers", async ({ page }, testInfo) => {
  await page.getByRole("tab", { name: "Body", exact: true }).click();
  await page.getByRole("tab", { name: "JSON", exact: true }).click();

  const contentType = page.getByLabel("请求 Content-Type");
  await expect(contentType).toBeVisible();
  await expect(contentType).toHaveValue("application/json");
  await contentType.selectOption("application/vnd.api+json");

  await page.getByRole("tab", { name: /Headers/ }).click();
  await expect(page.getByLabel("Header 1 名称")).toHaveValue("Content-Type");
  await expect(page.getByLabel("Header 1 值")).toHaveValue("application/vnd.api+json");

  if (testInfo.project.name === "desktop-chromium") {
    await page.getByRole("tab", { name: "Body", exact: true }).click();
    await page.locator(".http-request-tab-panel").screenshot({ path: "output/playwright/request-content-type.png" });
  }
});
