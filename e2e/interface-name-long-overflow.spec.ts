import { expect, test } from "@playwright/test";

test("contains a long saved interface name without covering lifecycle tabs", async ({ page }) => {
  await page.setViewportSize({ width: 900, height: 700 });
  await page.addInitScript(() => localStorage.clear());
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");

  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-request", { detail: {
    id: "saved-interface-name-long-overflow",
    name: "Saved",
    protocolId: "http",
    target: "https://api.example.com/users",
    payload: { type: "http", method: "GET", headers: [], body: null },
  } })));

  const header = page.locator(".interface-lifecycle-header");
  const tabs = header.locator(".interface-lifecycle-tabs");
  const slot = header.locator(".interface-lifecycle-name-slot");
  const control = slot.locator(".interface-lifecycle-name-control");
  const input = control.locator("input");
  await expect(slot).toHaveClass(/is-saved/);
  await input.fill("a".repeat(160));

  await expect.poll(() => input.evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
  }))).toEqual(expect.objectContaining({
    clientWidth: expect.any(Number),
    scrollWidth: expect.any(Number),
  }));

  const [headerBox, tabsBox, slotBox, controlBox, inputBox] = await Promise.all([
    header.boundingBox(),
    tabs.boundingBox(),
    slot.boundingBox(),
    control.boundingBox(),
    input.boundingBox(),
  ]);
  for (const box of [headerBox, tabsBox, slotBox, controlBox, inputBox]) expect(box).not.toBeNull();
  expect(slotBox!.x).toBeGreaterThanOrEqual(tabsBox!.x + tabsBox!.width);
  expect(controlBox!.x + controlBox!.width).toBeLessThanOrEqual(slotBox!.x + slotBox!.width);
  expect(inputBox!.x + inputBox!.width).toBeLessThanOrEqual(controlBox!.x + controlBox!.width);
  expect(slotBox!.x + slotBox!.width).toBeLessThanOrEqual(headerBox!.x + headerBox!.width);

  const metrics = await input.evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
  }));
  expect(metrics.scrollWidth).toBeGreaterThan(metrics.clientWidth);

  await header.screenshot({ path: "output/playwright/interface-name-long-overflow/header-900px.png" });
});
