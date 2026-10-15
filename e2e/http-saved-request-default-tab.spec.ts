import { expect, test } from "@playwright/test";
import { mkdir } from "node:fs/promises";

const baseRequest = {
  method: "POST",
  headers: [["Content-Type", "application/json"]],
  timeoutMs: 30_000,
  variables: {},
  assertions: [],
  auth: null,
  followRedirects: true,
  retryMax: 0,
  retryBackoffMs: 250,
  proxy: null,
  tlsVerify: true,
};

test("selects Params or Body from the content of an opened saved request", async ({ page }) => {
  await page.addInitScript(() => localStorage.clear());
  await page.route("http://127.0.0.1:39217/**", (route) => route.abort());
  await page.goto("/#workbench=http");
  await expect(page.getByRole("tablist", { name: "请求配置" })).toBeVisible();

  await page.evaluate((request) => window.dispatchEvent(new CustomEvent("protovoy-open-request", { detail: { protocolId: "http", request } })), {
    ...baseRequest,
    id: "body-default-tab",
    name: "Body default tab",
    url: "https://api.example.com/orders",
    body: '{"limit":10}',
  });
  await expect(page.locator("#http-request-tab-body")).toHaveAttribute("aria-selected", "true");

  await page.evaluate((request) => window.dispatchEvent(new CustomEvent("protovoy-open-request", { detail: { protocolId: "http", request } })), {
    ...baseRequest,
    id: "params-default-tab",
    name: "Params default tab",
    url: "https://api.example.com/orders?status=pending",
    body: '{"limit":10}',
  });
  await expect(page.locator("#http-request-tab-params")).toHaveAttribute("aria-selected", "true");

  await mkdir("output/playwright", { recursive: true });
  await page.locator(".http-request-tabs").screenshot({ path: "output/playwright/http-saved-request-default-tab.png" });
});
