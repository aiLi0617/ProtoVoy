import { expect, test } from "@playwright/test";

test("highlights new interface names and fits saved names to their text", async ({ page }) => {
  await page.addInitScript(() => localStorage.clear());
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");

  const header = page.locator(".interface-lifecycle-header");
  const slot = header.locator(".interface-lifecycle-name-slot");
  const input = slot.locator("input");
  const newNameColors = await input.evaluate((element) => ({
    background: getComputedStyle(element).backgroundColor,
    headerBackground: getComputedStyle(element.closest(".interface-lifecycle-header")!).backgroundColor,
  }));
  expect(newNameColors.background).not.toBe(newNameColors.headerBackground);

  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-request", { detail: {
    id: "saved-interface-name-refinement",
    name: "Saved",
    protocolId: "http",
    target: "https://api.example.com/users",
    payload: { type: "http", method: "GET", headers: [], body: null },
  } })));
  await expect(slot).toHaveClass(/is-saved/);
  await input.fill("Saved");

  await expect.poll(async () => input.evaluate((element) => {
    const style = getComputedStyle(element);
    const canvas = document.createElement("canvas");
    const context = canvas.getContext("2d")!;
    context.font = style.font;
    const textWidth = context.measureText(element.value).width;
    return element.getBoundingClientRect().width - textWidth;
  })).toBeLessThanOrEqual(3);
});
