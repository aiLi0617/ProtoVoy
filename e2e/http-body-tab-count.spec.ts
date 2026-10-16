import { expect, test } from "@playwright/test";
import { mkdir } from "node:fs/promises";

test("shows one raw request body beside the Body tab", async ({ page }) => {
  await page.addInitScript(() => localStorage.clear());
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");
  await expect(page.getByRole("tablist", { name: "\u8bf7\u6c42\u914d\u7f6e" })).toBeVisible();

  await expect.poll(async () => {
    await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-request", { detail: {
      protocolId: "http",
      request: {
        id: "body-count-preview",
        name: "Body count preview",
        method: "POST",
        url: "https://api.example.com/purchase-demand",
        headers: [["Content-Type", "application/json"], ["Accept", "application/json"]],
        body: JSON.stringify({ purchaseDemandId: 2110313839101628418, units: [] }, null, 2),
        timeoutMs: 30000,
        variables: {},
        assertions: [],
        auth: null,
        followRedirects: true,
        retryMax: 0,
        retryBackoffMs: 250,
        proxy: null,
        tlsVerify: true,
      },
    } })));
    return page.locator("#http-request-tab-body").textContent();
  }).toContain("1");

  await expect(page.locator("#http-request-tab-body")).toContainText("1");
  await expect(page.locator("#http-request-tab-headers")).toContainText("2");

  await mkdir("output/playwright", { recursive: true });
  await page.locator(".http-request-tabs").screenshot({ path: "output/playwright/http-body-count.png" });
});
