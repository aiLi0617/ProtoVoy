import { expect, test } from "@playwright/test";

test("shows collection progress and cancels the active run", async ({ page }) => {
  let runId = "";
  let cancelledRunId = "";
  let releaseRun: (() => void) | undefined;
  const runReleased = new Promise<void>((resolve) => { releaseRun = resolve; });

  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (pathname === "/health") return route.fulfill({ json: { service: "protovoy-agent", protocolApiVersion: "1" } });
    if (pathname === "/v1/session") return route.fulfill({ json: { token: "test-session" } });
    if (pathname === "/v1/workspace-tree") return route.fulfill({ json: {
      workspaces: [{ id: "default-workspace", name: "Local" }],
      projects: [{ id: "default-project", name: "Personal", workspaceId: "default-workspace" }],
      modules: [],
      collections: [{ id: "default-collection", name: "Smoke", projectId: "default-project", parentId: null, sortOrder: 0 }],
      requests: [],
    } });
    if (pathname === "/v1/environments") return route.fulfill({ json: [] });
    if (pathname === "/v1/collections/default-collection/runs") {
      runId = String(request.postDataJSON().runId);
      await runReleased;
      return route.fulfill({ json: [] });
    }
    if (pathname === `/v1/collection-runs/${runId}`) {
      return route.fulfill({ json: { runId, completed: 1, total: 2, state: "running" } });
    }
    if (pathname === `/v1/collection-runs/${runId}/cancel`) {
      cancelledRunId = runId;
      releaseRun?.();
      return route.fulfill({ status: 202 });
    }
    return route.fulfill({ status: 204 });
  });

  await page.goto("/#workbench=runner");
  await page.getByRole("button", { name: "运行当前集合" }).click();
  await expect(page.locator(".collection-runner-progress")).toContainText("1/2");
  await page.screenshot({ path: "output/playwright/collection-run-progress.png", fullPage: true });
  await page.getByRole("button", { name: "取消运行" }).click();
  await expect.poll(() => cancelledRunId).toBe(runId);
});
