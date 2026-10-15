import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");
});

test("traps focus in execution history and restores its opener", async ({ page }) => {
  const opener = page.getByTestId("command-trigger");
  await opener.focus();
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-focus-history")));

  const dialog = page.getByRole("dialog", { name: "执行历史" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("button", { name: "关闭执行历史" })).toBeFocused();

  const lastButton = dialog.getByRole("button").last();
  await lastButton.focus();
  await page.keyboard.press("Tab");
  await expect(dialog.getByRole("button", { name: "关闭执行历史" })).toBeFocused();

  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(opener).toBeFocused();
});
