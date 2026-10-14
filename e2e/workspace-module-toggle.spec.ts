import { expect, test } from "@playwright/test";

const labels = {
  expandAll: "\u5c55\u5f00\u6a21\u5757 API \u7684\u5168\u90e8\u76ee\u5f55",
  collapseAll: "\u6536\u8d77\u6a21\u5757 API \u7684\u5168\u90e8\u76ee\u5f55",
  expandCases: "\u5c55\u5f00\u8c03\u8bd5\u7528\u4f8b Request",
  collapseCases: "\u6536\u8d77\u8c03\u8bd5\u7528\u4f8b Request",
  collapseRoot: "\u6298\u53e0\u96c6\u5408 Root",
} as const;

test.beforeEach(async ({ page }) => {
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const { pathname } = new URL(route.request().url());
    const headers = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*" };
    if (route.request().method() === "OPTIONS") return route.fulfill({ status: 204, headers });
    if (pathname === "/health") return route.fulfill({ headers, json: { service: "protovoy-agent", version: "0.2.0", agentVersion: "0.2.0", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false } });
    if (pathname === "/v1/session") return route.fulfill({ headers, json: { token: "ui-preview-session" } });
    if (pathname === "/v1/workspace-tree") return route.fulfill({ headers, json: {
      workspaces: [{ id: "default-workspace", name: "Local workspace" }],
      projects: [{ id: "default-project", workspaceId: "default-workspace", name: "Module toggle test" }],
      modules: [{ id: "m1", projectId: "default-project", name: "API", isDefault: true }],
      collections: [
        { id: "root", projectId: "default-project", moduleId: "m1", name: "Root", parentId: null, sortOrder: 0 },
        { id: "child", projectId: "default-project", moduleId: "m1", name: "Child", parentId: "root", sortOrder: 0 },
      ],
      requests: [
        { id: "request-1", projectId: "default-project", collectionId: "child", name: "Request", protocolId: "http", method: "GET", target: "https://example.test" },
        { id: "case-1", projectId: "default-project", collectionId: "child", name: "Debug case", protocolId: "http", method: "GET", target: "https://example.test", envelope: { variables: { __protovoyCaseOf: "request-1" }, metadata: { __protovoyCaseType: "debug" } } },
      ],
    } });
    return route.fulfill({ status: 404, headers, json: {} });
  });
  await page.setViewportSize({ width: 1100, height: 760 });
  await page.goto("/#workbench=http");
});

test("distinguishes modules and expands directories without changing debug-case state", async ({ page }, testInfo) => {
  await expect(page.locator(".tree-module-icon")).toBeVisible();
  await expect(page.locator(".tree-collection-main svg").first()).toBeVisible();

  await page.getByRole("button", { name: labels.collapseCases }).click();
  await page.getByRole("button", { name: labels.collapseRoot }).click();
  await page.getByRole("button", { name: labels.expandAll }).click();
  await expect(page.getByRole("button", { name: labels.expandCases })).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByRole("button", { name: labels.collapseAll })).toBeVisible();
  await page.screenshot({ path: `output/playwright/workspace-module-expanded-${testInfo.project.name}.png` });

  await page.getByRole("button", { name: labels.collapseAll }).click();
  await page.getByRole("button", { name: labels.expandAll }).click();
  await expect(page.getByRole("button", { name: labels.expandCases })).toHaveAttribute("aria-expanded", "false");
});
