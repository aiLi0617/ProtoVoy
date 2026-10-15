import { expect, test } from "@playwright/test";

function envelope(id: string, name: string, variables: Record<string, string> = {}) {
  return {
    id, name, protocolId: "http", target: "https://example.test/quotes", environmentRef: null, authRef: null,
    timeoutMs: 30000, retryPolicy: { max_retries: 0, backoff_ms: 0 }, proxy: null,
    tls: { verify: true, client_cert_ref: null }, metadata: {},
    payload: { type: "http", method: "POST", headers: [], body: null, followRedirects: true },
    preScripts: [], postScripts: [], assertions: [], variables,
  };
}

test("edits only the case name and selects the parent interface after navigation", async ({ page }) => {
  const parent = envelope("quote-interface", "提交报价");
  const debugCase = envelope("quote-case", "直接提交", { __protovoyCaseOf: "quote-interface" });
  await page.addInitScript(() => localStorage.clear());
  await page.route("http://127.0.0.1:39217/**", async (route) => {
    const { pathname } = new URL(route.request().url());
    const headers = { "Access-Control-Allow-Origin": "*", "Access-Control-Allow-Headers": "*" };
    if (route.request().method() === "OPTIONS") return route.fulfill({ status: 204, headers });
    if (pathname === "/health") return route.fulfill({ headers, json: { service: "protovoy-agent", protocolApiVersion: "1", minProtocolApiVersion: "1", maxProtocolApiVersion: "1", authRequired: false } });
    if (pathname === "/v1/session") return route.fulfill({ headers, json: { token: "case-header-session" } });
    if (pathname === "/v1/requests/quote-interface") return route.fulfill({ headers, json: { id: parent.id, target: parent.target, envelope: parent } });
    if (pathname === "/v1/requests/quote-case") return route.fulfill({ headers, json: { id: debugCase.id, target: debugCase.target, envelope: debugCase } });
    if (pathname === "/v1/workspace-tree") return route.fulfill({ headers, json: {
      workspaces: [{ id: "workspace", name: "Local" }],
      projects: [{ id: "project", workspaceId: "workspace", name: "Quotes" }],
      modules: [{ id: "module", projectId: "project", name: "API", isDefault: true }],
      collections: [{ id: "collection", projectId: "project", moduleId: "module", name: "报价", parentId: null, sortOrder: 0 }],
      requests: [
        { id: parent.id, projectId: "project", collectionId: "collection", name: parent.name, protocolId: "http", method: "POST", target: parent.target, envelope: parent },
        { id: debugCase.id, projectId: "project", collectionId: "collection", name: debugCase.name, protocolId: "http", method: "POST", target: debugCase.target, envelope: debugCase },
      ],
    } });
    return route.fulfill({ status: 404, headers, json: {} });
  });
  await page.goto("/#workbench=http");

  const debugCaseTreeItem = page.locator('[data-request-id="quote-case"] .tree-request-main');
  if (!await debugCaseTreeItem.isVisible()) await page.getByRole("button", { name: "资源管理器" }).click();
  await debugCaseTreeItem.click();
  const header = page.locator(".interface-case-header");
  const interfaceLink = header.getByRole("button", { name: "提交报价" });
  const caseName = header.getByRole("textbox", { name: "用例名称" });
  await expect(interfaceLink).toBeVisible();
  await expect(caseName).toHaveValue("直接提交");
  await expect(page.getByText("继承接口结构，独立保存参数值、前后置操作与断言")).toHaveCount(0);
  await expect(page.locator(".interface-lifecycle-commandbar .http-request-name-field")).toHaveCount(0);

  await caseName.fill("修改后的用例");
  await expect(interfaceLink).toHaveText("提交报价");
  await interfaceLink.click();

  await expect(page.locator(".interface-lifecycle-name-slot input[aria-label='接口名称']")).toHaveValue("提交报价");
  await expect(page.locator('[data-request-id="quote-interface"]')).toHaveClass(/is-active/);
});
