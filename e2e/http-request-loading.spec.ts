import { expect, test } from "@playwright/test";

test("shows request progress until the HTTP execution completes", async ({ page }, testInfo) => {
  const cors = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*", "Access-Control-Allow-Methods": "GET,POST,OPTIONS" };
  let finishExecution!: () => void;
  const executionFinished = new Promise<void>((resolve) => { finishExecution = resolve; });
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (request.method() === "OPTIONS") return route.fulfill({ status: 204, headers: cors });
    if (pathname === "/health") return route.fulfill({ headers: cors, json: { service: "protovoy-agent", version: "test", agentVersion: "test", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false } });
    if (pathname === "/v1/session") return route.fulfill({ headers: cors, json: { token: "e2e-session-token" } });
    if (pathname === "/v1/executions") return route.fulfill({ headers: cors, json: { executionId: "loading-state-test", state: "running" } });
    if (pathname === "/v1/executions/loading-state-test/events") {
      await executionFinished;
      const now = new Date().toISOString();
      return route.fulfill({ headers: { ...cors, "Content-Type": "text/event-stream" }, body: `data: ${JSON.stringify({ type: "completed", summary: { executionId: "loading-state-test", requestId: "loading-request", protocolId: "http", state: "completed", startedAt: now, finishedAt: now, durationMs: 24, bytesReceived: 0, status: 204 } })}\n\n` });
    }
    if (pathname === "/v1/history" || pathname === "/v1/cookies") return route.fulfill({ headers: cors, json: [] });
    return route.abort();
  });

  await page.goto("/#workbench=http");
  await page.getByLabel("目标 URL").fill("https://example.test/loading");
  await page.getByRole("checkbox", { name: "校验响应" }).uncheck({ force: true });
  await page.getByRole("button", { name: /Send|发送/ }).click();

  const cancelButton = page.getByRole("button", { name: /Cancel|取消/ });
  await expect(cancelButton).toHaveAttribute("aria-busy", "true");
  await expect(cancelButton.locator(".request-loading-spinner")).toBeVisible();
  await expect(cancelButton).toBeEnabled();
  if (testInfo.project.name === "desktop-chromium") await cancelButton.screenshot({ path: "output/playwright/http-request-loading.png" });

  finishExecution();
  await expect(page.getByRole("button", { name: /Send|发送/ })).toBeVisible();
});
