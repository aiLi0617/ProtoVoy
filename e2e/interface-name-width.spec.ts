import { expect, test } from "@playwright/test";
import { mkdir } from "node:fs/promises";

test("sizes the lifecycle interface name for unsaved and saved requests", async ({ page }) => {
  await page.addInitScript(() => localStorage.clear());
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");

  const header = page.locator(".interface-lifecycle-header");
  const nameSlot = header.locator(".interface-lifecycle-name-slot");
  const nameInput = nameSlot.locator("input[aria-label='接口名称']");

  await expect(nameSlot).toHaveClass(/is-unsaved/);
  const unsavedHeaderBox = await header.boundingBox();
  const unsavedNameBox = await nameInput.boundingBox();
  expect(unsavedHeaderBox).not.toBeNull();
  expect(unsavedNameBox).not.toBeNull();
  expect(Math.abs(unsavedNameBox!.x + unsavedNameBox!.width - unsavedHeaderBox!.x - unsavedHeaderBox!.width)).toBeLessThanOrEqual(1);

  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-request", { detail: {
    id: "saved-interface-name-width",
    name: "查询用户列表",
    protocolId: "http",
    target: "https://api.example.com/users",
    payload: { type: "http", method: "GET", headers: [], body: null },
  } })));
  await expect(nameSlot).toHaveClass(/is-saved/);
  await nameInput.fill("查询用户列表");
  await expect(nameInput).toHaveValue("查询用户列表");

  const savedNameBox = await nameInput.boundingBox();
  expect(savedNameBox).not.toBeNull();
  expect(savedNameBox!.width).toBeLessThan(unsavedNameBox!.width);

  const longName = "这是一个用于验证超长接口名称保持单行并在输入框内部滚动的接口名称".repeat(4);
  await nameInput.fill(longName);
  const longNameBox = await nameInput.boundingBox();
  const longNameMetrics = await nameInput.evaluate((input) => ({
    clientWidth: input.clientWidth,
    scrollWidth: input.scrollWidth,
    whiteSpace: getComputedStyle(input).whiteSpace,
  }));
  const savedHeaderBox = await header.boundingBox();
  expect(longNameBox).not.toBeNull();
  expect(savedHeaderBox).not.toBeNull();
  expect(longNameBox!.x + longNameBox!.width).toBeLessThanOrEqual(savedHeaderBox!.x + savedHeaderBox!.width + 1);
  expect(longNameMetrics.scrollWidth).toBeGreaterThan(longNameMetrics.clientWidth);
  expect(longNameMetrics.whiteSpace).toBe("nowrap");

  await mkdir("output/playwright", { recursive: true });
  await page.screenshot({ path: "output/playwright/interface-name-width.png", fullPage: true });
});
