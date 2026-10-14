import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const { pathname } = new URL(route.request().url());
    const headers = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*" };
    if (route.request().method() === "OPTIONS") return route.fulfill({ status: 204, headers });
    if (pathname === "/health") return route.fulfill({ headers, json: { service: "protovoy-agent", version: "0.2.0", agentVersion: "0.2.0", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false } });
    if (pathname === "/v1/session") return route.fulfill({ headers, json: { token: "ui-preview-session" } });
    if (pathname === "/v1/workspace-tree") return route.fulfill({ headers, json: {
      workspaces: [{ id: "default-workspace", name: "Local workspace" }],
      projects: [{ id: "default-project", workspaceId: "default-workspace", name: "Layering test" }],
      modules: [{ id: "m1", projectId: "default-project", name: "API", isDefault: true }],
      collections: [{ id: "default-collection", projectId: "default-project", moduleId: "m1", name: "Tests", parentId: null, sortOrder: 0 }],
      requests: [],
    } });
    return route.fulfill({ status: 404, headers, json: {} });
  });
  await page.setViewportSize({ width: 800, height: 700 });
  await page.goto("/#workbench=http");
});

test("workspace create menu does not intersect the response divider", async ({ page }, testInfo) => {
  await page.locator(".workspace-create-trigger").click();
  const menu = page.locator("[data-workspace-create-menu]");
  const separator = page.locator(".workbench-panel:not([hidden]) .split-handle").first();
  await expect(menu).toBeVisible();

  const [menuBounds, separatorBounds] = await Promise.all([menu.boundingBox(), separator.boundingBox()]);
  expect(menuBounds).not.toBeNull();
  expect(separatorBounds).not.toBeNull();
  expect(menuBounds!.y + menuBounds!.height).toBeLessThanOrEqual(separatorBounds!.y - 7);
  await page.screenshot({ path: `output/playwright/workspace-menu-geometry-${testInfo.project.name}.png` });
});
