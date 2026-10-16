import { expect, test } from "@playwright/test";

const text = {
  createDirectory: "\u65b0\u5efa\u76ee\u5f55",
  directoryName: "\u76ee\u5f55\u540d\u79f0",
  parentDirectory: "\u7236\u7ea7\u76ee\u5f55",
  chooseParent: "\u9009\u62e9\u7236\u7ea7\u76ee\u5f55",
  module: "\u6a21\u5757",
  directory: "\u76ee\u5f55",
} as const;

test.beforeEach(async ({ page }) => {
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const { pathname } = new URL(route.request().url());
    const headers = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*" };
    if (route.request().method() === "OPTIONS") return route.fulfill({ status: 204, headers });
    if (pathname === "/health") return route.fulfill({ headers, json: { service: "protovoy-agent", version: "0.1.0", agentVersion: "0.1.0", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false } });
    if (pathname === "/v1/session") return route.fulfill({ headers, json: { token: "ui-preview-session" } });
    if (pathname === "/v1/workspace-tree") return route.fulfill({ headers, json: {
      workspaces: [{ id: "workspace", name: "Local workspace" }],
      projects: [{ id: "project", workspaceId: "workspace", name: "Directory picker test" }],
      modules: [{ id: "api", projectId: "project", name: "API", isDefault: true }, { id: "users", projectId: "project", name: "Users", isDefault: false }],
      collections: [{ id: "tests", projectId: "project", moduleId: "api", name: "Tests", parentId: null, sortOrder: 0 }, { id: "smoke", projectId: "project", moduleId: "api", name: "Smoke", parentId: "tests", sortOrder: 0 }],
      requests: [],
    } });
    return route.fulfill({ status: 404, headers, json: {} });
  });
  await page.setViewportSize({ width: 800, height: 700 });
  await page.goto("/#workbench=http");
});

test("searches, collapses, and selects a module or directory parent", async ({ page }, testInfo) => {
  await page.locator(".workspace-create-trigger").click();
  await page.getByRole("menuitem", { name: text.createDirectory, exact: true }).click();

  const dialog = page.getByRole("dialog", { name: text.createDirectory });
  const nameField = dialog.getByLabel(text.directoryName);
  const parentPicker = dialog.getByRole("combobox", { name: text.parentDirectory });
  await expect(nameField).toBeVisible();
  await expect(parentPicker).toHaveValue(`${text.module}\uff1aAPI`);
  expect(await nameField.evaluate((element) => Boolean(element.compareDocumentPosition(document.getElementById("collection-parent-label")) & Node.DOCUMENT_POSITION_FOLLOWING))).toBe(true);
  await expect(dialog.getByText("\u65b0\u76ee\u5f55\u5c06\u4f4d\u4e8e\u7b2c")).toHaveCount(0);

  await parentPicker.click();
  const tree = dialog.getByRole("tree", { name: text.chooseParent });
  await expect(tree).toBeVisible();
  await expect(tree.getByRole("treeitem")).toHaveCount(3);
  await expect(tree.getByRole("button", { name: "Tests", exact: true })).toBeVisible();
  await expect(tree.getByRole("button", { name: "Smoke", exact: true })).toHaveCount(0);

  await tree.getByRole("button", { name: "\u5c55\u5f00\u76ee\u5f55 Tests" }).click();
  await expect(tree.getByRole("button", { name: "Smoke", exact: true })).toBeVisible();
  await tree.getByRole("button", { name: "\u6536\u8d77\u6a21\u5757 API" }).click();
  await expect(tree.getByRole("button", { name: "Tests", exact: true })).toHaveCount(0);

  const search = dialog.getByRole("combobox", { name: text.parentDirectory });
  await expect(search).toHaveAttribute("placeholder", "搜索模块或目录");
  await expect(dialog.locator(".collection-parent-control")).toHaveClass(/is-open/);
  await expect(dialog.getByRole("button", { name: "收起父级目录" })).toBeVisible();
  await search.fill("Smoke");
  await expect(tree.getByRole("treeitem")).toHaveCount(3);
  await expect(tree.getByRole("treeitem", { name: /API/ })).toBeVisible();
  await expect(tree.getByRole("button", { name: "Tests", exact: true })).toBeVisible();
  await expect(tree.getByRole("button", { name: "Smoke", exact: true })).toBeVisible();
  await expect(tree.getByRole("treeitem", { name: /Users/ })).toHaveCount(0);
  await page.screenshot({ path: `output/playwright/create-directory-parent-search-${testInfo.project.name}.png` });

  await tree.getByRole("button", { name: "Smoke", exact: true }).click();
  await expect(parentPicker).toHaveValue(`${text.directory}\uff1aSmoke`);
  await parentPicker.click();
  await expect(dialog.getByRole("tree", { name: text.chooseParent }).getByRole("button", { name: "Smoke", exact: true })).toBeVisible();
  await page.screenshot({ path: `output/playwright/create-directory-parent-${testInfo.project.name}.png` });
});