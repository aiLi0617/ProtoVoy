import { expect, test } from "@playwright/test";

test("captures the collection environment control", async ({ page }) => {
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname === "/health") return route.fulfill({ json: { service: "protovoy-agent", protocolApiVersion: "1" } });
    if (pathname === "/v1/session") return route.fulfill({ json: { token: "test-session" } });
    if (pathname === "/v1/workspace-tree") return route.fulfill({ json: {
      workspaces: [{ id: "default-workspace", name: "Local" }],
      projects: [{ id: "default-project", name: "Personal", workspaceId: "default-workspace" }],
      modules: [],
      collections: [{ id: "default-collection", name: "Smoke", projectId: "default-project", parentId: null, sortOrder: 0 }],
      requests: [],
    } });
    if (pathname === "/v1/environments") return route.fulfill({ json: [
      { id: "env-staging", projectId: "default-project", name: "Staging", variables: {}, secretRefs: [], updatedAt: "2026-10-14T00:00:00Z" },
    ] });
    return route.fulfill({ status: 204 });
  });

  await page.goto("/#workbench=runner");
  const environment = page.getByLabel("集合运行环境");
  await expect(environment).toBeVisible();
  await environment.selectOption("env-staging");
  await page.screenshot({ path: "output/playwright/collection-runner-environment.png", fullPage: true });
});
