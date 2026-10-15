import { expect, test } from "@playwright/test";

test("keeps a focused saved interface name from overlapping its caret or neighbors", async ({ page }) => {
  await page.addInitScript(() => localStorage.clear());
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");
  const slot = page.locator(".interface-lifecycle-name-slot");
  const control = slot.locator(".interface-lifecycle-name-control");
  const input = slot.locator("input");

  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-request", { detail: {
    id: "saved-interface-name-overlap",
    name: "Saved",
    protocolId: "http",
    target: "https://api.example.com/users",
    payload: { type: "http", method: "GET", headers: [], body: null },
  } })));
  await expect(slot).toHaveClass(/is-saved/);
  await input.fill("保存接口名称");

  await expect.poll(() => input.evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
  }))).toEqual(expect.objectContaining({
    clientWidth: expect.any(Number),
    scrollWidth: expect.any(Number),
  }));
  const inputMetrics = await input.evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
  }));
  expect(inputMetrics.clientWidth).toBeGreaterThanOrEqual(inputMetrics.scrollWidth);
  const boxes = await Promise.all([control.boundingBox(), input.boundingBox()]);
  expect(boxes[0]).not.toBeNull();
  expect(boxes[1]).not.toBeNull();
  expect(boxes[1]!.x).toBeGreaterThanOrEqual(boxes[0]!.x);
  expect(boxes[1]!.x + boxes[1]!.width).toBeLessThanOrEqual(boxes[0]!.x + boxes[0]!.width);
  await expect(slot).toHaveCSS("padding-right", "6px");
});
