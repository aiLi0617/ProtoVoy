import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.route("http://127.0.0.1:39217/**", route => route.abort());
});

test("links Mock field errors to their controls", async ({ page }) => {
  await page.goto("/#workbench=mock");
  await page.getByRole("button", { name: "新建场景" }).click();
  const ipSettings = page.getByRole("region", { name: "IP 条件设置" });
  await ipSettings.locator("label.ui-switch").click();
  const ip = page.getByLabel("客户端 IP");
  await expect(ip).toHaveAttribute("aria-describedby", "mock-client-ip-error");
  await expect(page.locator("#mock-client-ip-error")).toBeVisible();

  await page.getByRole("tab", { name: "设置" }).click();
  const status = page.getByLabel("HTTP 状态码");
  await status.fill("99");
  await expect(status).toHaveAttribute("aria-invalid", "true");
  const descriptionId = await status.getAttribute("aria-describedby");
  expect(descriptionId).toBeTruthy();
  await expect(page.locator(`#${descriptionId}`)).toContainText("请输入 100–999");
  await page.locator(".mock-response-editor").screenshot({ path: "output/playwright/field-errors-mock.png" });
});

test("links an invalid UDP target to its recovery message", async ({ page }) => {
  await page.goto("/#workbench=udp");
  const target = page.getByLabel("UDP 目标地址");
  await target.fill("invalid-target");
  await expect(target).toHaveAttribute("aria-invalid", "true");
  await expect(target).toHaveAttribute("aria-describedby", "udp-target-error");
  await expect(page.locator("#udp-target-error")).toBeVisible();
});
