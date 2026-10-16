import { expect, test } from "@playwright/test";

test("saves typed global parameters and creates isolated environment drafts", async ({ page }) => {
  const cors = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*", "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS" };
  const environments: Array<Record<string, unknown>> = [];
  let createdPayload: Record<string, unknown> | null = null;

  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === "OPTIONS") return route.fulfill({ status: 204, headers: cors });
    if (path === "/v1/environments/default") {
      return route.fulfill({ headers: cors, json: { variables: {}, globalParameters: [], secretRefs: [] } });
    }
    if (path === "/v1/environments" && request.method() === "GET") return route.fulfill({ headers: cors, json: environments });
    if (path === "/v1/environments" && request.method() === "POST") {
      createdPayload = JSON.parse(request.postData() ?? "{}");
      const created = { id: "env-created", updatedAt: new Date().toISOString(), ...createdPayload };
      environments.push(created);
      return route.fulfill({ status: 201, headers: cors, json: created });
    }
    return route.abort();
  });

  await page.goto("/");
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-environment")));
  const dialog = page.locator(".environment-dialog");
  await dialog.getByRole("button", { name: "全局参数" }).click();
  await dialog.getByLabel("新Header全局参数名称").fill("X-Client-ID");
  const headerRow = dialog.locator(".environment-parameter-row.has-content").last();
  await headerRow.locator('input:not([type="checkbox"])').nth(1).fill("{{client_id}}");
  await dialog.getByRole("tab", { name: /Query/ }).click();
  await expect(dialog.getByLabel("新Query全局参数名称")).toHaveValue("");
  await dialog.getByRole("tab", { name: /Header/ }).click();
  await expect(dialog.getByLabel("全局参数名称 1")).toHaveValue("X-Client-ID");
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect(dialog.getByRole("button", { name: "保存", exact: true })).toBeDisabled();
  await dialog.getByRole("button", { name: "新建环境" }).click();
  await dialog.getByLabel("环境名称").fill("Development");
  const variableRow = dialog.locator(".environment-kv-row").first();
  await variableRow.locator("input").nth(0).fill("base_url");
  await variableRow.locator("input").nth(1).fill("https://dev.example.com");
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect.poll(() => createdPayload).toMatchObject({ name: "Development", variables: { base_url: "https://dev.example.com" } });

  await dialog.getByRole("button", { name: "新建环境" }).click();
  await expect(dialog.getByLabel("环境名称")).toHaveValue("");
  await expect(dialog.getByLabel("新变量名称")).toHaveValue("");
  await expect.poll(() => dialog.locator("input").evaluateAll((inputs) => inputs.some((input) => input.value === "https://dev.example.com"))).toBe(false);
});
