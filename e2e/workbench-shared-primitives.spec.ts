import { expect, test } from "@playwright/test";
import { mkdir } from "node:fs/promises";

test.beforeEach(async ({ page }) => {
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");
});

test("http command bar uses shared controls", { tag: "@protocol-workbenches" }, async ({ page }) => {
  const commandBar = page.locator(".http-request-commandbar:visible");
  await expect(commandBar).toBeVisible();
  await expect(commandBar.locator(".ui-button.http-send-button").first()).toBeVisible();
  await mkdir("output/playwright", { recursive: true });
  await page.screenshot({ path: "output/playwright/http-shared-commandbar.png", fullPage: true });
});

test("protocol layout localizes shared controls", { tag: "@protocol-workbenches" }, async ({ page }) => {
  await page.evaluate(() => localStorage.setItem("protovoy:locale", "en-US"));
  await page.reload();
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-select-workbench", { detail: "mqtt" })));
  await expect(page.getByLabel("Interface name")).toBeVisible();
  await expect(page.getByRole("region", { name: "Request configuration", exact: true })).toBeVisible();
  await expect(page.getByRole("region", { name: "Response inspector", exact: true })).toBeVisible();
  await mkdir("output/playwright", { recursive: true });
  await page.screenshot({ path: "output/playwright/mqtt-english-shared-layout.png", fullPage: true });
});

for (const workbench of [
  { id: "grpc", commandBar: ".grpc-commandbar" },
  { id: "websocket", commandBar: ".websocket-commandbar" },
  { id: "sse", commandBar: ".sse-target-row" },
  { id: "tcp", commandBar: ".websocket-commandbar" },
  { id: "udp", commandBar: ".websocket-commandbar" },
] as const) {
  test(`${workbench.id} command bar uses shared controls`, { tag: "@protocol-workbenches" }, async ({ page }) => {
    await page.evaluate((id) => window.dispatchEvent(new CustomEvent("protovoy-select-workbench", { detail: id })), workbench.id);
    const commandBar = page.locator(`${workbench.commandBar}:visible`);
    await expect(commandBar).toBeVisible();
    await expect(commandBar.locator(".ui-input").first()).toBeVisible();
    await expect(commandBar.locator(".ui-button").first()).toBeVisible();
    await mkdir("output/playwright", { recursive: true });
    await page.screenshot({ path: `output/playwright/${workbench.id}-shared-commandbar.png`, fullPage: true });
  });
}


test("script library uses shared controls", { tag: "@protocol-workbenches" }, async ({ page }) => {
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-script-library")));
  const library = page.locator(".script-library:visible");
  await expect(library).toBeVisible();
  await expect(library.locator(".ui-input").first()).toBeVisible();
  await expect(library.locator(".ui-icon-button").first()).toBeVisible();
  await expect(library.locator(".ui-button").first()).toBeVisible();
  await mkdir("output/playwright", { recursive: true });
  await page.screenshot({ path: "output/playwright/script-library-shared-controls.png", fullPage: true });
});
