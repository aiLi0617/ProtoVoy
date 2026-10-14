import { expect, test } from "@playwright/test";

test("shows module actions and renames the module", async ({ page }, testInfo) => {
  let moduleName = "API";
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const { pathname } = new URL(route.request().url());
    const headers = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*" };
    if (route.request().method() === "OPTIONS") return route.fulfill({ status: 204, headers });
    if (pathname === "/health") return route.fulfill({ headers, json: { service: "protovoy-agent", version: "0.2.0", agentVersion: "0.2.0", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false } });
    if (pathname === "/v1/session") return route.fulfill({ headers, json: { token: "ui-preview-session" } });
    if (pathname === "/v1/modules/m1" && route.request().method() === "PATCH") {
      moduleName = (route.request().postDataJSON() as { name: string }).name;
      return route.fulfill({ status: 204, headers });
    }
    if (pathname === "/v1/workspace-tree") return route.fulfill({ headers, json: {
      workspaces: [{ id: "default-workspace", name: "Local workspace" }],
      projects: [{ id: "default-project", workspaceId: "default-workspace", name: "Module menu test" }],
      modules: [{ id: "m1", projectId: "default-project", name: moduleName, isDefault: true }],
      collections: [{ id: "root", projectId: "default-project", moduleId: "m1", name: "Root", parentId: null, sortOrder: 0 }],
      requests: [],
    } });
    return route.fulfill({ status: 404, headers, json: {} });
  });

  await page.setViewportSize({ width: 1100, height: 760 });
  await page.goto("/#workbench=http");
  await page.getByRole("button", { name: "API \u6a21\u5757\u66f4\u591a\u64cd\u4f5c" }).click();
  await expect(page.getByRole("menuitem", { name: "\u65b0\u5efa\u76ee\u5f55" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "\u91cd\u547d\u540d" })).toBeEnabled();
  await page.screenshot({ path: `output/playwright/workspace-module-menu-${testInfo.project.name}.png` });

  await page.getByRole("menuitem", { name: "\u91cd\u547d\u540d" }).click();
  await expect(page.getByRole("heading", { name: "\u91cd\u547d\u540d\u6a21\u5757" })).toBeVisible();
  await page.getByLabel("\u8f93\u5165\u5185\u5bb9").fill("API Core");
  await page.getByRole("button", { name: "\u786e\u8ba4", exact: true }).click();
  await expect(page.getByRole("treeitem", { name: "\u6a21\u5757 API Core" })).toBeVisible();
});
