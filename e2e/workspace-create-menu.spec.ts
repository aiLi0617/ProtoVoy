import { expect, test } from "@playwright/test";

const labels = {
  close: "\u5173\u95ed",
  createInterface: "\u65b0\u5efa\u63a5\u53e3",
  createModule: "\u65b0\u5efa\u6a21\u5757",
  createDirectory: "\u65b0\u5efa\u76ee\u5f55",
  directoryName: "\u76ee\u5f55\u540d\u79f0",
  parentDirectory: "\u7236\u7ea7\u76ee\u5f55",
  more: "\u66f4\u591a\u2026",
} as const;

test.beforeEach(async ({ page }) => {
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const { pathname } = new URL(route.request().url());
    const headers = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*" };
    if (route.request().method() === "OPTIONS") return route.fulfill({ status: 204, headers });
    if (pathname === "/health") return route.fulfill({ headers, json: { service: "protovoy-agent", version: "0.1.0", agentVersion: "0.1.0", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false } });
    if (pathname === "/v1/session") return route.fulfill({ headers, json: { token: "ui-preview-session" } });
    if (pathname === "/v1/workspace-tree") return route.fulfill({ headers, json: {
      workspaces: [{ id: "default-workspace", name: "Local workspace" }],
      projects: [{ id: "default-project", workspaceId: "default-workspace", name: "Create menu test" }],
      modules: [{ id: "m1", projectId: "default-project", name: "API", isDefault: true }],
      collections: [{ id: "default-collection", projectId: "default-project", moduleId: "m1", name: "Tests", parentId: null, sortOrder: 0 }],
      requests: [],
    } });
    return route.fulfill({ status: 404, headers, json: {} });
  });
  await page.setViewportSize({ width: 800, height: 700 });
  await page.goto("/#workbench=http");
});

test("keeps resources visible while more protocols expand in the two-column menu", async ({ page }, testInfo) => {
  const trigger = page.locator(".workspace-create-trigger");
  await trigger.click();

  const menu = page.locator("[data-workspace-create-menu]");
  await expect(menu).toBeVisible();
  const menuBounds = await menu.boundingBox();
  expect(menuBounds).not.toBeNull();
  expect(menuBounds!.width).toBeLessThanOrEqual(340);
  expect(menuBounds!.height).toBeLessThanOrEqual(300);
  await expect(menu.getByText(labels.createInterface, { exact: true })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: labels.createModule })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: labels.createDirectory, exact: true })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: "TCP", exact: true })).toHaveCount(0);
  await page.screenshot({ path: `output/playwright/workspace-create-menu-${testInfo.project.name}.png` });

  await menu.getByRole("menuitem", { name: labels.more }).click();
  await expect(menu.getByRole("menuitem", { name: "TCP", exact: true })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: "Redis", exact: true })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: labels.createModule })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: labels.createDirectory, exact: true })).toBeVisible();
  await page.screenshot({ path: `output/playwright/workspace-create-menu-expanded-${testInfo.project.name}.png` });

  await menu.getByRole("menuitem", { name: labels.createModule }).click();
  await expect(page.getByRole("heading", { name: labels.createModule })).toBeVisible();
  await page.getByRole("button", { name: labels.close, exact: true }).click();

  await trigger.click();
  await page.getByRole("menuitem", { name: labels.createDirectory, exact: true }).click();
  await expect(page.getByRole("heading", { name: labels.createDirectory })).toBeVisible();
  await expect(page.getByLabel(labels.directoryName)).toBeVisible();
  await expect(page.getByRole("combobox", { name: labels.parentDirectory })).toBeVisible();
  await expect(page.getByText("\u65b0\u76ee\u5f55\u5c06\u4f4d\u4e8e\u7b2c")).toHaveCount(0);
});
