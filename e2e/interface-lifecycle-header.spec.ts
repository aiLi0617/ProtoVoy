import { expect, test } from "@playwright/test";
import { mkdir } from "node:fs/promises";

test("moves the interface name after lifecycle tabs and keeps the tabs text-only", async ({ page }) => {
  await page.addInitScript(() => localStorage.clear());
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");
  await expect(page.getByRole("tablist", { name: "接口生命周期" })).toBeVisible();

  const lifecycleTabs = page.locator(".interface-lifecycle-tabs");
  const docsTab = lifecycleTabs.getByRole("tab", { name: "文档预览" });
  const mockTab = lifecycleTabs.getByRole("tab", { name: "Mock" });
  const nameInput = page.locator(".interface-lifecycle-name-slot input[aria-label='接口名称']");

  await expect(nameInput).toHaveCount(1);
  await expect(page.locator(".interface-lifecycle-commandbar .http-request-name-field")).toHaveCount(0);
  await expect(lifecycleTabs.locator("button svg")).toHaveCount(0);
  const newDocsBox = await docsTab.boundingBox();
  const newNameBox = await nameInput.boundingBox();
  expect(newDocsBox).not.toBeNull();
  expect(newNameBox).not.toBeNull();
  expect(newNameBox!.x).toBeGreaterThanOrEqual(newDocsBox!.x + newDocsBox!.width);

  await expect.poll(async () => {
    await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-request", { detail: {
      id: "saved-interface-header",
      name: "查询用户列表",
      protocolId: "http",
      target: "https://api.example.com/users",
      payload: { type: "http", method: "GET", headers: [], body: null },
    } })));
    return mockTab.count();
  }).toBe(1);
  await nameInput.fill("查询用户列表");
  await expect(nameInput).toHaveValue("查询用户列表");

  const mockBox = await mockTab.boundingBox();
  const nameBox = await nameInput.boundingBox();
  expect(mockBox).not.toBeNull();
  expect(nameBox).not.toBeNull();
  expect(nameBox!.x).toBeGreaterThanOrEqual(mockBox!.x + mockBox!.width);

  await mkdir("output/playwright", { recursive: true });
  await page.screenshot({ path: "output/playwright/interface-name-after-mock.png", fullPage: true });
});
