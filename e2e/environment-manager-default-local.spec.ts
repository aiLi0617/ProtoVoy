import { expect, test } from "@playwright/test";

test("keeps global variables separate in navigation and lists the default local environment", async ({ page }) => {
  const cors = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*", "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS" };
  const globalConfiguration = {
    id: "default-env",
    projectId: "default-project",
    name: "Default",
    variables: { global_token: "shared" },
    globalParameters: [{ location: "header", name: "X-Client", parameterType: "string", value: "ProtoVoy", enabled: true, required: false, description: "" }],
    secretRefs: [],
    updatedAt: new Date().toISOString(),
  };
  const localEnvironment = {
    id: "local-env",
    projectId: "default-project",
    name: "\u672c\u5730\u73af\u5883",
    variables: { base_url: "http://127.0.0.1:3000" },
    globalParameters: [],
    secretRefs: [],
    updatedAt: new Date().toISOString(),
  };
  let savedLocal: Record<string, unknown> | null = null;
  let savedGlobal = false;

  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === "OPTIONS") return route.fulfill({ status: 204, headers: cors });
    if (path === "/health") return route.fulfill({ headers: cors, json: { service: "protovoy-agent", version: "test", agentVersion: "test", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false } });
    if (path === "/v1/session") return route.fulfill({ headers: cors, json: { token: "test-session" } });
    if (path === "/v1/environments/default" && request.method() === "GET") return route.fulfill({ headers: cors, json: globalConfiguration });
    if (path === "/v1/environments/default" && request.method() === "PUT") {
      savedGlobal = true;
      return route.fulfill({ headers: cors, json: globalConfiguration });
    }
    if (path === "/v1/environments/local-env" && request.method() === "PUT") {
      savedLocal = JSON.parse(request.postData() ?? "{}");
      return route.fulfill({ headers: cors, json: { ...localEnvironment, ...savedLocal } });
    }
    if (path === "/v1/environments" && request.method() === "GET") return route.fulfill({ headers: cors, json: [globalConfiguration, localEnvironment] });
    return route.abort();
  });

  await page.goto("/");
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-environment")));
  const dialog = page.locator(".environment-dialog");
  const globalSection = dialog.locator(".environment-manager-nav section").first();
  const environmentSection = dialog.locator(".environment-list");

  await expect(globalSection.getByRole("button", { name: "\u5168\u5c40\u53d8\u91cf" })).toBeVisible();
  await expect(environmentSection.getByRole("button", { name: "\u672c\u5730\u73af\u5883" })).toBeVisible();

  await environmentSection.getByRole("button", { name: "\u672c\u5730\u73af\u5883" }).click();
  await expect(dialog.getByRole("heading", { name: "\u672c\u5730\u73af\u5883" })).toBeVisible();
  await expect(dialog.getByLabel("\u73af\u5883\u540d\u79f0")).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "\u5220\u9664\u73af\u5883" })).toHaveCount(0);
  await page.screenshot({ path: "output/playwright/environment-manager-default-local.png", fullPage: true });

  await dialog.getByLabel("\u65b0\u53d8\u91cf\u540d\u79f0").fill("api_version");
  const apiVersionRow = dialog.locator(".environment-kv-row.has-content").last();
  await apiVersionRow.locator("input").nth(1).fill("v1");
  await dialog.getByRole("button", { name: "\u4fdd\u5b58", exact: true }).click();
  await expect.poll(() => savedLocal).toMatchObject({
    variables: { base_url: "http://127.0.0.1:3000", api_version: "v1" },
  });
  expect(savedGlobal).toBe(false);
});
