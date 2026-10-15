import { expect, test } from "@playwright/test";

test("overrides a collection run with the selected environment", async ({ page }) => {
  let runBody: unknown;
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
    if (pathname === "/v1/collections/default-collection/runs") {
      runBody = route.request().postDataJSON();
      return route.fulfill({ json: [] });
    }
    return route.fulfill({ status: 204 });
  });

  await page.goto("/#workbench=runner");
  await page.getByLabel("集合运行环境").selectOption("env-staging");
  await page.getByRole("button", { name: "运行当前集合" }).click();

  await expect.poll(() => runBody).toEqual(expect.objectContaining({ failFast: false, environmentId: "env-staging", runId: expect.any(String) }));
});
