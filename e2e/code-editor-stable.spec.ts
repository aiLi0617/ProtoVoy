import { expect, test } from "@playwright/test";

test("reveals a fully rendered response editor without leaving the fallback mounted", async ({ page }, testInfo) => {
  await page.addInitScript(() => {
    localStorage.setItem("protovoy:agent-url", location.origin);
    localStorage.removeItem("protovoy:agent-token");
  });
  const jsonHeaders = { "content-type": "application/json" };
  const responseBody = JSON.stringify({ code: 401, data: {}, msg: "账号未登录", toast: null }, null, 2);

  await page.route("http://127.0.0.1:5180/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/health") return route.fulfill({ headers: jsonHeaders, body: JSON.stringify({ service: "protovoy-agent", version: "test", agentVersion: "test", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false }) });
    if (path === "/v1/session") return route.fulfill({ headers: jsonHeaders, body: JSON.stringify({ token: "test" }) });
    if (path === "/v1/executions") return route.fulfill({ headers: jsonHeaders, body: JSON.stringify({ executionId: "stable-editor-e2e", state: "running" }) });
    if (path === "/v1/executions/stable-editor-e2e/events") {
      const now = new Date().toISOString();
      const events = [
        { type: "response_meta", status: 401, statusText: "Unauthorized", headers: [["content-type", "application/json"]], contentType: "application/json", sizeHint: responseBody.length },
        { type: "response_chunk", contentType: "application/json", size: responseBody.length, preview: responseBody, done: true },
        { type: "completed", summary: { executionId: "stable-editor-e2e", requestId: "request-e2e", protocolId: "http", state: "completed", startedAt: now, finishedAt: now, durationMs: 12, bytesReceived: responseBody.length, status: 401 } },
      ];
      return route.fulfill({ headers: { "content-type": "text/event-stream" }, body: events.map((event) => `data: ${JSON.stringify(event)}\n\n`).join("") });
    }
    if (path === "/v1/history/stable-editor-e2e/body") return route.fulfill({ headers: jsonHeaders, body: responseBody });
    if (path === "/v1/history" || path === "/v1/cookies") return route.fulfill({ headers: jsonHeaders, body: "[]" });
    return route.continue();
  });

  await page.goto("/#workbench=http");
  await page.getByLabel("目标 URL").fill("https://example.test/session");
  await page.getByRole("checkbox", { name: "校验响应" }).uncheck({ force: true });
  await page.getByRole("button", { name: "Send", exact: true }).click();

  const responseEditor = page.locator(".http-response-scroll.is-body .code-editor");
  await expect(responseEditor).toBeVisible();
  await expect(responseEditor.locator(".monaco-editor")).toBeVisible();
  await expect(responseEditor.locator(".code-editor-fallback")).toHaveCount(0);
  await expect(responseEditor).toContainText("账号未登录");
  if (testInfo.project.name === "desktop-chromium") await responseEditor.screenshot({ path: "output/playwright/http-response-editor-stable.png" });
});
