import { expect, test } from "@playwright/test";

test("preserves HTTP variable templates and the selected environment", async ({ page }) => {
  const cors = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*", "Access-Control-Allow-Methods": "GET,POST,OPTIONS" };
  let submitted: Record<string, unknown> | undefined;
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (request.method() === "OPTIONS") return route.fulfill({ status: 204, headers: cors });
    if (pathname === "/health") return route.fulfill({ headers: cors, json: { service: "protovoy-agent", version: "test", agentVersion: "test", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false } });
    if (pathname === "/v1/session") return route.fulfill({ headers: cors, json: { token: "e2e-session-token" } });
    if (pathname === "/v1/environments") return route.fulfill({ headers: cors, json: [
      { id: "default-env", name: "Default", variables: {}, secretRefs: [] },
      { id: "env-staging", name: "Staging", variables: {}, secretRefs: [] },
    ] });
    if (pathname === "/v1/executions" && request.method() === "POST") {
      submitted = request.postDataJSON() as Record<string, unknown>;
      return route.fulfill({ headers: cors, json: { executionId: "http-variable-inputs", state: "running" } });
    }
    if (pathname === "/v1/executions/http-variable-inputs/events") {
      const now = new Date().toISOString();
      return route.fulfill({ headers: { ...cors, "Content-Type": "text/event-stream" }, body: `data: ${JSON.stringify({ type: "completed", summary: { executionId: "http-variable-inputs", requestId: "http-variable-request", protocolId: "http", state: "completed", startedAt: now, finishedAt: now, durationMs: 1, bytesReceived: 0, status: 204 } })}\n\n` });
    }
    if (pathname === "/v1/history" || pathname === "/v1/cookies") return route.fulfill({ headers: cors, json: [] });
    return route.abort();
  });

  await page.goto("/#workbench=http");
  await page.getByLabel("请求环境").selectOption("env-staging");
  await page.getByLabel("HTTP 方法").selectOption("POST");
  await page.getByLabel("目标 URL").fill("https://api.example.test/items");

  await page.getByRole("tab", { name: "Params", exact: true }).click();
  await page.getByLabel("Param 1 名称").fill("account");
  await page.getByLabel("Param 1 值").fill("{{accountId}}");

  await page.getByRole("tab", { name: "Headers", exact: true }).click();
  await page.getByLabel("Header 1 名称").fill("X-Trace-Id");
  await page.getByLabel("Header 1 值").fill("{{traceId}}");

  await page.getByRole("tab", { name: "Body", exact: true }).click();
  await page.getByRole("tab", { name: "x-www-form-urlencoded", exact: true }).click();
  await page.getByLabel("表单字段 1 名称").fill("owner");
  await page.getByLabel("表单字段 1 值").fill("{{ownerId}}");
  await page.getByRole("checkbox", { name: "校验响应" }).uncheck({ force: true });
  await page.getByRole("button", { name: /Send|发送/ }).click();

  await expect.poll(() => submitted).toBeDefined();
  expect(submitted).toMatchObject({
    environmentRef: "env-staging",
    target: "https://api.example.test/items?account={{accountId}}",
    payload: {
      type: "http",
      method: "POST",
      headers: expect.arrayContaining([["X-Trace-Id", "{{traceId}}"]]),
      body: "owner={{ownerId}}",
    },
  });
});
