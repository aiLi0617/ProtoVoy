import { expect, test } from "@playwright/test";

test("default 200 validation reports a failed 500 response without saved response definitions", async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem("protovoy:agent-url", location.origin);
    localStorage.removeItem("protovoy:agent-token");
  });
  const json = { "content-type": "application/json" };

  await page.route("http://127.0.0.1:5180/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/health") {
      return route.fulfill({ headers: json, body: JSON.stringify({ service: "protovoy-agent", version: "test", agentVersion: "test", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false }) });
    }
    if (path === "/v1/session") return route.fulfill({ headers: json, body: JSON.stringify({ token: "test" }) });
    if (path === "/v1/executions") return route.fulfill({ headers: json, body: JSON.stringify({ executionId: "default-status-e2e", state: "running" }) });
    if (path === "/v1/executions/default-status-e2e/events") {
      const now = new Date().toISOString();
      const summary = { executionId: "default-status-e2e", requestId: "request-e2e", protocolId: "http", state: "completed", startedAt: now, finishedAt: now, durationMs: 12, bytesReceived: 2, status: 500 };
      const events = [
        { type: "response_meta", status: 500, statusText: "Internal Server Error", headers: [["content-type", "application/json"]], contentType: "application/json", sizeHint: 2 },
        { type: "response_chunk", contentType: "application/json", size: 2, preview: "{}", done: true },
        { type: "completed", summary },
      ];
      return route.fulfill({ headers: { "content-type": "text/event-stream" }, body: events.map((event) => `data: ${JSON.stringify(event)}\n\n`).join("") });
    }
    if (path === "/v1/history/default-status-e2e/body") return route.fulfill({ headers: json, body: "{}" });
    if (path === "/v1/history") return route.fulfill({ headers: json, body: "[]" });
    if (path === "/v1/cookies") return route.fulfill({ headers: json, body: "[]" });
    return route.continue();
  });

  await page.goto("/#workbench=http");
  await page.getByLabel("目标 URL").fill("https://example.test/error");
  await page.getByRole("button", { name: "Send", exact: true }).click();

  await expect(page.getByText("接口设计 · HTTP 状态码")).toBeVisible();
  await expect(page.getByText("失败 1")).toBeVisible();
  await expect(page.getByText("实际状态码与所选返回响应不一致")).toBeVisible();
});
