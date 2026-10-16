import { expect, test } from "@playwright/test";

test("matches inline parameter creation and persists a new environment", async ({ page }) => {
  const cors = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*", "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS" };
  const environments: Array<Record<string, unknown>> = [];
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === "OPTIONS") return route.fulfill({ status: 204, headers: cors });
    if (path === "/health") return route.fulfill({ headers: cors, json: { service: "protovoy-agent", version: "test", agentVersion: "test", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false } });
    if (path === "/v1/session") return route.fulfill({ headers: cors, json: { token: "test-session" } });
    if (path === "/v1/environments/default") return route.fulfill({ headers: cors, json: { id: "default-env", projectId: "default-project", name: "Default", variables: {}, globalParameters: [], secretRefs: [], updatedAt: new Date().toISOString() } });
    if (path === "/v1/environments" && request.method() === "GET") return route.fulfill({ headers: cors, json: environments });
    if (path === "/v1/environments" && request.method() === "POST") {
      const payload = JSON.parse(request.postData() ?? "{}") as Record<string, unknown>;
      const created = { id: "env-development", updatedAt: new Date().toISOString(), ...payload };
      environments.push(created);
      return route.fulfill({ status: 201, headers: cors, json: created });
    }
    return route.abort();
  });

  await page.goto("/");
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-environment")));
  let dialog = page.locator(".environment-dialog");
  await dialog.getByRole("button", { name: "\u5168\u5c40\u53c2\u6570" }).click();
  await expect(dialog.locator(".environment-parameter-header > span")).toHaveText(["\u53c2\u6570\u540d", "\u7c7b\u578b", "\u9ed8\u8ba4\u503c", "\u9ed8\u8ba4\u542f\u7528", "\u8bf4\u660e", ""]);

  const addParameterRow = dialog.locator(".environment-parameter-row.is-new");
  await expect(addParameterRow.locator("input").first()).toHaveAttribute("placeholder", "\u6dfb\u52a0\u53c2\u6570");
  await addParameterRow.hover();
  await expect(addParameterRow.getByLabel(/\u5168\u5c40\u53c2\u6570\u7c7b\u578b/)).toBeVisible();
  await dialog.getByLabel("\u65b0Header\u5168\u5c40\u53c2\u6570\u540d\u79f0").fill("X-Complete-Header");

  const parameterRow = dialog.locator(".environment-parameter-row.has-content").last();
  await expect(parameterRow.locator("input").first()).toHaveValue("X-Complete-Header");
  await parameterRow.getByLabel(/\u5168\u5c40\u53c2\u6570\u7c7b\u578b/).selectOption("number");
  const requiredButton = parameterRow.getByRole("button", { name: /\u8bbe\u4e3a\u5fc5\u586b/ });
  await expect(requiredButton).toHaveText("*");
  await requiredButton.click();
  await expect(parameterRow.getByRole("button", { name: /\u53d6\u6d88\u5fc5\u586b/ })).toHaveAttribute("aria-pressed", "true");
  await parameterRow.locator("input").nth(1).fill("42");
  await parameterRow.getByRole("switch").click();
  await expect(parameterRow.getByRole("switch")).toHaveAttribute("aria-checked", "false");
  await expect(dialog.locator(".environment-parameter-row.is-new input").first()).toHaveAttribute("placeholder", "\u6dfb\u52a0\u53c2\u6570");

  await dialog.getByRole("button", { name: "\u65b0\u5efa\u73af\u5883" }).click();
  await page.getByRole("button", { name: "\u653e\u5f03\u5e76\u65b0\u5efa" }).click();
  await dialog.getByLabel("\u73af\u5883\u540d\u79f0").fill("Development");
  await dialog.getByLabel("\u65b0\u53d8\u91cf\u540d\u79f0").fill("base_url");
  const variableRow = dialog.locator(".environment-kv-row.has-content").last();
  await variableRow.locator("input").nth(1).fill("https://dev.example.com");
  await dialog.getByRole("button", { name: "\u4fdd\u5b58", exact: true }).click();
  await expect(dialog.getByRole("status")).toContainText("\u914d\u7f6e\u5df2\u4fdd\u5b58");
  await dialog.getByRole("button", { name: "\u5173\u95ed\u73af\u5883\u7ba1\u7406" }).click();

  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-environment")));
  dialog = page.locator(".environment-dialog");
  await dialog.getByRole("button", { name: "Development" }).click();
  await expect(dialog.locator('input[value="https://dev.example.com"]')).toBeVisible();
});
