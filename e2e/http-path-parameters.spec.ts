import { expect, test } from "@playwright/test";

test("derives Path parameters from URL placeholders and resolves them for sending", { tag: "@http" }, async ({ page }) => {
  const cors = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*", "Access-Control-Allow-Methods": "GET,POST,OPTIONS" };
  let submitted: Record<string, unknown> | undefined;
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (request.method() === "OPTIONS") return route.fulfill({ status: 204, headers: cors });
    if (pathname === "/health") return route.fulfill({ headers: cors, json: { service: "protovoy-agent", version: "test", agentVersion: "test", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false } });
    if (pathname === "/v1/session") return route.fulfill({ headers: cors, json: { token: "e2e-session-token" } });
    if (pathname === "/v1/environments") return route.fulfill({ headers: cors, json: [] });
    if (pathname === "/v1/executions" && request.method() === "POST") {
      submitted = request.postDataJSON() as Record<string, unknown>;
      return route.fulfill({ headers: cors, json: { executionId: "http-path-parameters", state: "running" } });
    }
    if (pathname === "/v1/executions/http-path-parameters/events") {
      const now = new Date().toISOString();
      return route.fulfill({ headers: { ...cors, "Content-Type": "text/event-stream" }, body: `data: ${JSON.stringify({ type: "completed", summary: { executionId: "http-path-parameters", requestId: "path-request", protocolId: "http", state: "completed", startedAt: now, finishedAt: now, durationMs: 1, bytesReceived: 0, status: 204 } })}\n\n` });
    }
    if (pathname === "/v1/history" || pathname === "/v1/cookies") return route.fulfill({ headers: cors, json: [] });
    return route.abort();
  });

  await page.goto("/#workbench=http");
  await page.getByLabel("目标 URL").fill("https://api.example.test/quotes/{quoteId}?source={queryValue}");

  await expect(page.getByText("Path 参数", { exact: true })).toBeVisible();
  await expect(page.getByLabel("Path 参数 1 名称")).toHaveValue("quoteId");
  await page.getByLabel("Path 参数 1 值").fill("quote 42");
  await page.screenshot({ path: "output/playwright/http-path-parameters.png", fullPage: true });

  await page.getByRole("checkbox", { name: "校验响应" }).uncheck({ force: true });
  await page.getByRole("button", { name: /Send|发送/ }).click();

  await expect.poll(() => submitted).toBeDefined();
  expect(submitted).toMatchObject({ target: "https://api.example.test/quotes/quote%2042?source={queryValue}" });
});
