import { expect, test } from "@playwright/test";

test("runs a collection through the Agent collection endpoint", async ({ page }) => {
  let runBody: unknown;
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname === "/health") {
      return route.fulfill({ json: { service: "protovoy-agent", protocolApiVersion: "1" } });
    }
    if (pathname === "/v1/session") {
      return route.fulfill({ json: { token: "test-session" } });
    }
    if (pathname === "/v1/workspace-tree") {
      return route.fulfill({ json: {
        workspaces: [{ id: "default-workspace", name: "Local" }],
        projects: [{ id: "default-project", name: "Personal", workspaceId: "default-workspace" }],
        modules: [],
        collections: [{ id: "default-collection", name: "Smoke", projectId: "default-project", parentId: null, sortOrder: 0 }],
        requests: [],
      } });
    }
    if (pathname === "/v1/collections/default-collection/runs") {
      runBody = route.request().postDataJSON();
      return route.fulfill({ json: [{ requestId: "request-1", name: "Health", protocolId: "http", passed: true, status: 200, durationMs: 8, error: null, failedAssertions: [] }] });
    }
    return route.fulfill({ status: 204 });
  });

  await page.goto("/#workbench=runner");
  await page.getByRole("button", { name: "\u8fd0\u884c\u5f53\u524d\u96c6\u5408" }).click();

  await expect(page.locator(".collection-runner-summary")).toContainText("1/1");
  expect(runBody).toEqual({ failFast: false, runId: expect.any(String) });
});
