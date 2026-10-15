import { expect, test } from "@playwright/test";

test("shows the saved interface name chrome only while interacting", async ({ page }) => {
  await page.addInitScript(() => localStorage.clear());
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");
  const slot = page.locator(".interface-lifecycle-name-slot");
  const input = slot.locator("input");

  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-request", { detail: {
    id: "saved-interface-name-hover",
    name: "Saved",
    protocolId: "http",
    target: "https://api.example.com/users",
    payload: { type: "http", method: "GET", headers: [], body: null },
  } })));
  await expect(slot).toHaveClass(/is-saved/);
  await input.fill("Saved");
  await page.locator(".interface-lifecycle-tabs").getByRole("tab", { name: "文档预览" }).click();
await page.mouse.move(1400, 800);

  await expect.poll(() => slot.evaluate((element) => getComputedStyle(element, "::after").backgroundColor)).toBe("rgba(0, 0, 0, 0)");
  const resting = await slot.evaluate((element) => getComputedStyle(element, "::after").backgroundColor);
  const restingWidth = await input.evaluate((element) => element.getBoundingClientRect().width);
  await input.hover();
  const hovered = await slot.evaluate((element) => getComputedStyle(element, "::after").backgroundColor);
  const hoveredWidth = await input.evaluate((element) => element.getBoundingClientRect().width);

  expect(resting).toBe("rgba(0, 0, 0, 0)");
  expect(hovered).not.toBe(resting);
  expect(hoveredWidth).toBe(restingWidth);
});
