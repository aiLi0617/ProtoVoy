import { expect, test } from "@playwright/test";

test("creates an environment without leaking its draft into the next one", async ({ page }) => {
  const cors = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*", "Access-Control-Allow-Methods": "GET, POST, PUT, DELETE, OPTIONS" };
  let createdPayload: Record<string, unknown> | null = null;
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === "OPTIONS") return route.fulfill({ status: 204, headers: cors });
    if (path === "/v1/environments/default") return route.fulfill({ headers: cors, json: { variables: {}, globalParameters: [], secretRefs: [] } });
    if (path === "/v1/environments" && request.method() === "GET") return route.fulfill({ headers: cors, json: [] });
    if (path === "/v1/environments" && request.method() === "POST") {
      createdPayload = JSON.parse(request.postData() ?? "{}");
      return route.fulfill({ status: 201, headers: cors, json: { id: "env-created", updatedAt: new Date().toISOString(), ...createdPayload } });
    }
    return route.abort();
  });

  await page.goto("/");
  await page.evaluate(() => window.dispatchEvent(new CustomEvent("protovoy-open-environment")));
  const dialog = page.locator(".environment-dialog");
  await dialog.getByRole("button", { name: "新建环境" }).click();
  await dialog.getByLabel("环境名称").fill("Development");
  await dialog.getByLabel("新变量名称").fill("base_url");
  await dialog.getByLabel("新变量值").fill("https://dev.example.com");
  await dialog.getByRole("button", { name: "添加变量" }).click();
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect.poll(() => createdPayload).toMatchObject({ name: "Development", variables: { base_url: "https://dev.example.com" } });

  await dialog.getByRole("button", { name: "新建环境" }).click();
  await expect(dialog.getByLabel("环境名称")).toHaveValue("");
  await expect(dialog.getByLabel("新变量名称")).toHaveValue("");
  await expect(page.getByDisplayValue("https://dev.example.com")).toHaveCount(0);
});
