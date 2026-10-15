import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");
});

test("uses distinct semantic icons for workbench commands", { tag: "@app-shell" }, async ({ page }, testInfo) => {
  await page.getByTestId("command-trigger").click();

  const commands = page.locator('#protovoy-command-results button[id^="command-workbench-"]');
  await expect(commands.first()).toBeVisible();

  const iconClasses = await commands.locator("svg").evaluateAll((icons) => icons.map((icon) => icon.getAttribute("class") ?? ""));
  expect(new Set(iconClasses).size).toBe(iconClasses.length);

  await page.screenshot({ path: testInfo.outputPath("workbench-command-icons.png") });
});
